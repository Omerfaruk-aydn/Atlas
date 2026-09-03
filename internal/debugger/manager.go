package debugger

import (
	"context"
	"sync"
)

// closer is the one method Manager needs from a session to evict it --
// deliberately smaller than the full Session API so tests can exercise
// Manager's bookkeeping (reuse-closes-the-old-one, get, close, closeAll)
// against a trivial fake instead of a real dlv process.
type closer interface {
	Close()
}

// Manager keeps at most one open debug session per chat session ID, so a
// second `start` replaces (and closes) whatever this chat session was
// already debugging rather than leaking it.
type Manager[S closer] struct {
	mu       sync.Mutex
	opts     Options
	start    func(ctx context.Context, opts Options, program string, args []string) (S, error)
	sessions map[string]S
}

func newManager[S closer](opts Options, start func(context.Context, Options, string, []string) (S, error)) *Manager[S] {
	return &Manager[S]{opts: opts, start: start, sessions: map[string]S{}}
}

var (
	defaultManager     *Manager[*Session]
	defaultManagerOnce sync.Once
)

// GetManager returns the process-wide debug session manager, built from
// opts the first time it's called. Later callers reuse the same manager
// (and its already-open sessions) regardless of what opts they pass --
// this mirrors browser.GetManager, for the same reason: it must survive
// the agent's tool list being reassembled mid-session.
func GetManager(opts Options) *Manager[*Session] {
	defaultManagerOnce.Do(func() {
		defaultManager = newManager[*Session](opts, Start)
	})
	return defaultManager
}

// Start launches a new debug session for id, closing whatever session was
// already open for it first -- one debug target per chat session at a
// time, matching a real debugger's single line of control.
func (m *Manager[S]) Start(ctx context.Context, id, program string, args []string) (S, error) {
	m.mu.Lock()
	existing, hadExisting := m.sessions[id]
	delete(m.sessions, id)
	opts := m.opts
	m.mu.Unlock()

	if hadExisting {
		existing.Close()
	}

	s, err := m.start(ctx, opts, program, args)
	if err != nil {
		var zero S
		return zero, err
	}

	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

// Get returns the open session for id, if any.
func (m *Manager[S]) Get(id string) (S, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Close closes and drops the session for id. Safe to call when none is
// open.
func (m *Manager[S]) Close(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if ok {
		s.Close()
	}
}

// CloseAll closes every open session. Called on process shutdown so a
// `dlv dap` process never outlives the agent that launched it.
func (m *Manager[S]) CloseAll() {
	m.mu.Lock()
	sessions := m.sessions
	m.sessions = map[string]S{}
	m.mu.Unlock()
	for _, s := range sessions {
		s.Close()
	}
}
