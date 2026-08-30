package tui

import (
	"sync"
	"time"
)

// SessionIDTracker is the dedicated session-id resolver. Hermes's
// agent's RPC promises an eventual "session id" — the App polls the
// backend until one arrives, then commits it to local state so the
// submit gate can open and messages can be tagged.
//
// Atlas's current agent emits the session id implicitly through
// EventApprovalRequest and EventTurnDone. The tracker here is a
// principled future home for the explicit polling pattern.
type SessionIDTracker struct {
	mu        sync.Mutex
	id        string
	resolvedAt time.Time
	promise   chan string
	clock     func() time.Time
}

const sessionIDTimeout = 4 * time.Second

// newSessionIDTracker returns an empty tracker with a fresh promise
// channel. Callers receive the id from .ID() once it's resolved;
// .Wait() blocks until either the id arrives or the timeout elapses.
func newSessionIDTracker() *SessionIDTracker {
	return &SessionIDTracker{
		promise: make(chan string, 1),
		clock:   time.Now,
	}
}

// Resolve commits a session id (e.g. when the agent's first event
// arrives). Idempotent: subsequent calls are no-ops so the tracker
// can be safely called from multiple event paths.
func (s *SessionIDTracker) Resolve(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.id != "" {
		return
	}
	s.id = id
	s.resolvedAt = s.clock()
	// Non-blocking send — if no one is waiting, the value is
	// stashed in s.id for ID() to read.
	select {
	case s.promise <- id:
	default:
	}
}

// ID returns the resolved session id, or "" if not yet resolved.
func (s *SessionIDTracker) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

// Wait blocks until the session id resolves or the timeout elapses.
// Returns the id (which may be "" on timeout). Mirrors Hermes's
// scheduleStartupPrompt polling pattern: poll the promise channel
// up to N×interval times.
func (s *SessionIDTracker) Wait() string {
	deadline := s.clock().Add(sessionIDTimeout)
	for s.clock().Before(deadline) {
		select {
		case id := <-s.promise:
			return id
		case <-time.After(100 * time.Millisecond):
			// retry; the promise will be filled by Resolve from
			// the agent's event goroutine.
		}
	}
	return s.ID()
}

// Resolved reports whether an id has been committed.
func (s *SessionIDTracker) Resolved() bool {
	return s.ID() != ""
}
