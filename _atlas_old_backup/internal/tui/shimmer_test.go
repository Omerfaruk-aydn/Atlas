package tui

import "testing"

// sharedShimmerClock broadcasts phase updates to all subscribers.
func TestShimmerClockBroadcasts(t *testing.T) {
	clock := newSharedShimmerClock(20)
	defer func() {
		// Stop the goroutine — there's no Stop() method, so just let
		// the test process exit; not a leak for a unit test.
	}()
	clock.Start()
	sub := clock.Subscribe()
	defer clock.Unsubscribe(sub)

	// Wait briefly for a tick.
	got := <-sub
	if got.now.IsZero() {
		t.Error("expected a non-zero phase update")
	}
}

// Shimmer View renders something at every width.
func TestShimmerViewRendersAtAnyWidth(t *testing.T) {
	clock := newSharedShimmerClock(20)
	clock.Start()
	s := NewShimmer(clock, 0)
	defer s.Close()
	for _, w := range []int{0, 1, 5, 20, 100} {
		got := s.View(w, func(s string) string { return s }, func(s string) string { return s })
		if w == 0 && got != "" {
			t.Errorf("width 0 should produce empty output, got %q", got)
		}
		if w > 0 && got == "" {
			t.Errorf("width %d should produce non-empty output", w)
		}
	}
}
