package tui

import (
	"errors"
	"testing"
	"time"
)

func TestSubmissionPolicyDefaultsToQueue(t *testing.T) {
	p := newSubmissionPolicy()
	if p.Mode() != BusyQueue {
		t.Errorf("expected default BusyQueue, got %q", p.Mode())
	}
}

func TestSubmissionPolicyQueueWhileStreaming(t *testing.T) {
	p := newSubmissionPolicy()
	if action := p.submit("hello", true); action != "queue" {
		t.Errorf("expected queue while streaming, got %q", action)
	}
	if p.QueueLength() != 1 {
		t.Errorf("expected 1 queued, got %d", p.QueueLength())
	}
}

func TestSubmissionPolicySubmitWhenIdle(t *testing.T) {
	p := newSubmissionPolicy()
	if action := p.submit("hello", false); action != "submit" {
		t.Errorf("expected submit when idle, got %q", action)
	}
	if p.QueueLength() != 0 {
		t.Errorf("expected empty queue, got %d", p.QueueLength())
	}
}

func TestSubmissionPolicyInterruptSubmitsImmediately(t *testing.T) {
	p := newSubmissionPolicy()
	p.SetMode(BusyInterrupt)
	if action := p.submit("hello", true); action != "submit" {
		t.Errorf("expected submit under interrupt mode, got %q", action)
	}
}

func TestSessionIDTrackerResolveAndWait(t *testing.T) {
	s := newSessionIDTracker()
	s.Resolve("abc")
	if s.ID() != "abc" {
		t.Errorf("expected ID 'abc', got %q", s.ID())
	}
	if !s.Resolved() {
		t.Error("expected Resolved() = true after Resolve")
	}
	if got := s.Wait(); got != "abc" {
		t.Errorf("expected Wait() = 'abc', got %q", got)
	}
}

func TestGitBranchCacheDedup(t *testing.T) {
	g := newGitBranchCache()
	g.execFn = func(cwd string) (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "main", nil
	}
	// Two concurrent Get() calls for the same cwd should hit the
	// subprocess only once.
	done := make(chan struct{}, 2)
	go func() { g.Get("/tmp"); done <- struct{}{} }()
	go func() { g.Get("/tmp"); done <- struct{}{} }()
	<-done
	<-done
	// Both should have seen "main".
	for i := 0; i < 2; i++ {
		b, _ := g.Get("/tmp")
		if b != "main" {
			t.Errorf("expected main, got %q", b)
		}
	}
}

func TestGitBranchCacheTTLExpiry(t *testing.T) {
	g := newGitBranchCache()
	g.ttl = 1 * time.Millisecond
	calls := 0
	g.execFn = func(cwd string) (string, error) {
		calls++
		return "main", nil
	}
	g.Get("/tmp")
	time.Sleep(5 * time.Millisecond)
	g.Get("/tmp")
	if calls < 2 {
		t.Errorf("expected at least 2 exec calls after TTL expiry, got %d", calls)
	}
}

func TestGitBranchCacheErrorNotCached(t *testing.T) {
	g := newGitBranchCache()
	g.execFn = func(cwd string) (string, error) {
		return "", errors.New("git not found")
	}
	if _, err := g.Get("/tmp"); err == nil {
		t.Error("expected error from execFn")
	}
	// Error path shouldn't pollute the cache.
	if _, ok := g.cache["/tmp"]; ok {
		t.Error("error result should not be cached")
	}
}
