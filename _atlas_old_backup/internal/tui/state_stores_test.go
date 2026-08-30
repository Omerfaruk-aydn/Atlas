package tui

import "testing"

func TestTurnStoreResetPreservesPendingNotice(t *testing.T) {
	ts := newTurnStore()
	ts.enqueueNotice(Notice{Key: "k", Text: "x", Kind: NoticeFlash, TTLMS: 1000})
	ts.resetTurnState()
	if ts.pendingNotice == nil {
		t.Error("resetTurnState must NOT clear pendingNotice (Hermes Strategy B contract)")
	}
}

func TestTurnStoreFlushReturnsAndClears(t *testing.T) {
	ts := newTurnStore()
	ts.enqueueNotice(Notice{Key: "k", Text: "hi", Kind: NoticeFlash})
	n := ts.flushPendingNotice()
	if n == nil || n.Text != "hi" {
		t.Errorf("expected notice 'hi', got %+v", n)
	}
	if ts.flushPendingNotice() != nil {
		t.Error("expected nil after flush")
	}
}

func TestTurnStorePatchIsolation(t *testing.T) {
	ts := newTurnStore()
	ts.patch(func(s *turnStore) { s.streamingBuf = "abc" })
	if ts.streamingBuf != "abc" {
		t.Errorf("expected streamingBuf 'abc', got %q", ts.streamingBuf)
	}
}

func TestUIStoreDefaultFocused(t *testing.T) {
	u := newUIStore()
	if !u.terminalFocused {
		t.Error("expected default terminal focused = true")
	}
}

func TestUIStorePatch(t *testing.T) {
	u := newUIStore()
	u.patch(func(s *uiStore) { s.busyMode = BusySteer })
	if u.busyMode != BusySteer {
		t.Errorf("expected BusySteer, got %q", u.busyMode)
	}
}
