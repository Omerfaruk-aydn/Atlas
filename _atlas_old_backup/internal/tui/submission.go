package tui

import (
	"errors"
	"sync"
)

// submissionState is the synchronous busy-flip state. The flip from
// idle→busy MUST happen before the async drop-detection RPC fires
// (which runs before the actual submit), otherwise a second rapid
// Enter during the async gap would race a second submit onto the
// backend instead of taking the local-enqueue path.
//
// This is the Atlas port of Hermes's submissionCore.submitPrompt —
// the actual RPC plumbing is in agent.Agent; the TUI just owns the
// gate that prevents the double-submit race.
type submissionState struct {
	mu     sync.Mutex
	busy   bool
	hasSID bool // whether a session ID has been resolved yet
}

// ErrSessionBusy is returned when the user tries to submit while a
// previous submit is still mid-flight. Matches Hermes's isSessionBusyError
// surface; the actual error messages come from the backend, this
// is just a typed local sentinel for the gate.
var ErrSessionBusy = errors.New("session busy")

// ErrSessionNotReady is returned when the user tries to submit before
// the agent has a session ID. Atlas never actually hits this path
// because the App doesn't accept input until the agent has resolved
// a session, but the gate exists for completeness (e.g. /model firing
// from a slash command).
var ErrSessionNotReady = errors.New("session not ready yet")

// TryClaim attempts to claim the submit gate. Returns nil on success
// (caller may now issue the async RPC), or one of the sentinel errors
// when the gate is already claimed. The caller MUST call Release
// after the submit completes (success, failure, or context cancel).
func (s *submissionState) TryClaim() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.hasSID {
		return ErrSessionNotReady
	}
	if s.busy {
		return ErrSessionBusy
	}
	s.busy = true
	return nil
}

// Release frees the submit gate. Idempotent: calling Release when the
// gate is already free is a no-op.
func (s *submissionState) Release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = false
}

// SetSessionReady marks the session ID as resolved, allowing future
// submits to claim the gate. Atlas calls this once at startup (or
// after the first turn's session creation).
func (s *submissionState) SetSessionReady() {
	s.mu.Lock()
	s.hasSID = true
	s.mu.Unlock()
}

// IsBusy returns the current gate state.
func (s *submissionState) IsBusy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy
}

// skipDetectDrop is the "literal submission" path: when the app
// starts with `-q "..."` (a startup query from the launcher), the
// prompt is sent verbatim without any drop-detection scan. The text
// is treated as opaque — no shell metacharacter escaping, no
// paste-detection, just a clean prompt.submit. This matches Hermes's
// `submitPrompt({skipDetectDrop:true})` parameter.
type skipDetectDrop struct{}

// startupQuery is a thin wrapper the App sets when launched with a
// `-q` startup query. The App's Init triggers a submit of this text
// once a session ID resolves.
type startupQuery struct {
	Text  string
	skipDetectDrop bool
}
