// Package browser drives a real Chrome/Chromium instance over the Chrome
// DevTools Protocol (via chromedp) so the agent's browser tool can navigate,
// click, type, run JavaScript, and take screenshots against a live page --
// the kind of UI verification a headless test suite can't do (visual review,
// exploring a site with no API, following a login flow interactively).
package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// Options configures how new sessions are launched and reaped. It is set
// once, on the manager's first use (see GetManager); later callers asking
// for a differently-configured manager still get the one already running.
type Options struct {
	// ExecutablePath is the Chrome/Chromium binary to launch. Empty lets
	// chromedp search the usual install locations.
	ExecutablePath string
	// Headless runs the browser without a visible window.
	Headless bool
	// ActionTimeout bounds a single action (navigate, click, eval...).
	ActionTimeout time.Duration
	// IdleTimeout is how long an unused session is kept open before a
	// later call reaps it. Zero disables reaping.
	IdleTimeout time.Duration
}

// Session drives one open browser tab, kept alive across calls so a
// multi-step flow (navigate, then click, then read the result) operates on
// the same page instead of a fresh one each time.
type Session interface {
	Navigate(url string) error
	Click(selector string) error
	Type(selector, text string) error
	PressKey(name string) error
	Eval(expression string) (string, error)
	Text(selector string) (string, error)
	HTML(selector string) (string, error)
	Screenshot(fullPage bool) ([]byte, error)
	URL() (string, error)
	// Close releases the underlying browser process. Safe to call more
	// than once.
	Close()
}

// namedKeys maps the tool's friendly key names to chromedp/kb's control
// character encoding for KeyEvent.
var namedKeys = map[string]string{
	"enter":      kb.Enter,
	"tab":        kb.Tab,
	"escape":     kb.Escape,
	"backspace":  kb.Backspace,
	"delete":     kb.Delete,
	"arrowup":    kb.ArrowUp,
	"arrowdown":  kb.ArrowDown,
	"arrowleft":  kb.ArrowLeft,
	"arrowright": kb.ArrowRight,
}

// ResolveKey translates a friendly key name (case-insensitive) into the
// encoding PressKey expects, and reports whether it was recognized.
func ResolveKey(name string) (string, bool) {
	k, ok := namedKeys[name]
	return k, ok
}

// SupportedKeys lists the key names ResolveKey accepts, for error messages.
func SupportedKeys() []string {
	names := make([]string, 0, len(namedKeys))
	for name := range namedKeys {
		names = append(names, name)
	}
	return names
}

type chromedpSession struct {
	ctx           context.Context
	cancel        context.CancelFunc
	actionTimeout time.Duration
	closeOnce     sync.Once
}

func newChromedpSession(opts Options) (Session, error) {
	allocOpts := append(append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...),
		chromedp.Flag("headless", opts.Headless),
	)
	if opts.ExecutablePath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(opts.ExecutablePath))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocOpts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	// Launch now so a missing browser binary or launch failure surfaces
	// here, at session creation, instead of on the caller's first action.
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		allocCancel()
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	return &chromedpSession{
		ctx:           ctx,
		cancel:        func() { cancel(); allocCancel() },
		actionTimeout: opts.ActionTimeout,
	}, nil
}

func (s *chromedpSession) run(actions ...chromedp.Action) error {
	timeout := s.actionTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()
	return chromedp.Run(runCtx, actions...)
}

func (s *chromedpSession) Navigate(url string) error {
	return s.run(chromedp.Navigate(url))
}

func (s *chromedpSession) Click(selector string) error {
	return s.run(chromedp.Click(selector, chromedp.ByQuery))
}

func (s *chromedpSession) Type(selector, text string) error {
	return s.run(
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

func (s *chromedpSession) PressKey(name string) error {
	key, ok := ResolveKey(name)
	if !ok {
		return fmt.Errorf("unsupported key %q (supported: %v)", name, SupportedKeys())
	}
	return s.run(chromedp.KeyEvent(key))
}

func (s *chromedpSession) Eval(expression string) (string, error) {
	var result string
	if err := s.run(chromedp.Evaluate(expression, &result)); err != nil {
		return "", err
	}
	return result, nil
}

func (s *chromedpSession) Text(selector string) (string, error) {
	var result string
	if err := s.run(chromedp.Text(selector, &result, chromedp.ByQuery)); err != nil {
		return "", err
	}
	return result, nil
}

func (s *chromedpSession) HTML(selector string) (string, error) {
	var result string
	if err := s.run(chromedp.OuterHTML(selector, &result, chromedp.ByQuery)); err != nil {
		return "", err
	}
	return result, nil
}

func (s *chromedpSession) Screenshot(fullPage bool) ([]byte, error) {
	var buf []byte
	if fullPage {
		if err := s.run(chromedp.FullScreenshot(&buf, 90)); err != nil {
			return nil, err
		}
		return buf, nil
	}
	if err := s.run(chromedp.CaptureScreenshot(&buf)); err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *chromedpSession) URL() (string, error) {
	var url string
	if err := s.run(chromedp.Location(&url)); err != nil {
		return "", err
	}
	return url, nil
}

func (s *chromedpSession) Close() {
	s.closeOnce.Do(s.cancel)
}

// newSessionFunc lets tests substitute a fake driver instead of launching a
// real browser process.
type newSessionFunc func(Options) (Session, error)

// Manager keeps at most one open Session per chat session ID, so a
// multi-step browser flow within a turn (and across turns of the same
// session) reuses the same tab instead of relaunching Chrome each call.
type Manager struct {
	mu         sync.Mutex
	opts       Options
	newSession newSessionFunc
	sessions   map[string]Session
	lastUsed   map[string]time.Time
}

func newManager(opts Options, newSession newSessionFunc) *Manager {
	return &Manager{
		opts:       opts,
		newSession: newSession,
		sessions:   map[string]Session{},
		lastUsed:   map[string]time.Time{},
	}
}

var (
	defaultManager     *Manager
	defaultManagerOnce sync.Once
)

// GetManager returns the process-wide browser manager, built from opts the
// first time it's called. Later callers reuse the same manager (and its
// already-open sessions) regardless of what opts they pass -- this mirrors
// shell.GetBackgroundShellManager, which the bash/job_output tools rely on
// to survive the agent's tool list being reassembled mid-session.
func GetManager(opts Options) *Manager {
	defaultManagerOnce.Do(func() {
		defaultManager = newManager(opts, newChromedpSession)
	})
	return defaultManager
}

// Session returns the open session for id, launching one if none exists.
func (m *Manager) Session(id string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.reapLocked()

	if s, ok := m.sessions[id]; ok {
		m.lastUsed[id] = time.Now()
		return s, nil
	}

	s, err := m.newSession(m.opts)
	if err != nil {
		return nil, err
	}
	m.sessions[id] = s
	m.lastUsed[id] = time.Now()
	return s, nil
}

// Close closes the session for id, if one is open. Safe to call when none
// is.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[id]; ok {
		s.Close()
		delete(m.sessions, id)
		delete(m.lastUsed, id)
	}
}

// CloseAll closes every open session. Called on process shutdown so a
// headless Chrome instance never outlives the agent that launched it.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		s.Close()
		delete(m.sessions, id)
		delete(m.lastUsed, id)
	}
}

// reapLocked closes and drops sessions idle for longer than opts.IdleTimeout.
// Called with mu held, on every Session lookup rather than off a background
// goroutine, so a manager nobody is using holds no timers.
func (m *Manager) reapLocked() {
	if m.opts.IdleTimeout <= 0 {
		return
	}
	cutoff := time.Now().Add(-m.opts.IdleTimeout)
	for id, last := range m.lastUsed {
		if last.Before(cutoff) {
			if s, ok := m.sessions[id]; ok {
				s.Close()
			}
			delete(m.sessions, id)
			delete(m.lastUsed, id)
		}
	}
}
