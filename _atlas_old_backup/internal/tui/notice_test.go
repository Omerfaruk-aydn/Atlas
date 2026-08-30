package tui

import (
	"testing"
	"time"
)

// newNoticeBoard with a controlled clock makes the TTL math
// deterministic.
func newTestBoard() (*NoticeBoard, *time.Time) {
	now := time.Unix(0, 0)
	board := newNoticeBoard(func() time.Time { return now })
	return board, &now
}

// A flash notice is visible immediately.
func TestNoticeBoardSetFlashImmediatelyVisible(t *testing.T) {
	board, _ := newTestBoard()
	board.set(Notice{Key: "k", Text: "hi", Kind: NoticeFlash, TTLMS: 5000})
	if got := board.currentText(); got != "hi" {
		t.Errorf("expected immediate visibility, got %q", got)
	}
}

// Sticky notice is preserved indefinitely (no TTL).
func TestNoticeBoardStickySurvivesPastTTL(t *testing.T) {
	board, now := newTestBoard()
	board.set(Notice{Key: "s", Text: "auth expired", Kind: NoticeSticky})
	*now = now.Add(10 * time.Minute)
	if got := board.currentText(); got != "auth expired" {
		t.Errorf("sticky notice must not auto-expire, got %q", got)
	}
}

// Flash notice expires after its TTL.
func TestNoticeBoardFlashExpires(t *testing.T) {
	board, now := newTestBoard()
	board.set(Notice{Key: "f", Text: "transient", Kind: NoticeFlash, TTLMS: 1000})
	*now = now.Add(2 * time.Second)
	if got := board.currentText(); got != "" {
		t.Errorf("expected flash to expire, got %q", got)
	}
}

// A flash never displaces a sticky notice.
func TestNoticeBoardFlashDoesNotDisplaceSticky(t *testing.T) {
	board, _ := newTestBoard()
	board.set(Notice{Key: "s", Text: "sticky!", Kind: NoticeSticky})
	board.set(Notice{Key: "f", Text: "flashy", Kind: NoticeFlash, TTLMS: 5000})
	if got := board.currentText(); got != "sticky!" {
		t.Errorf("sticky must survive a flash attempt, got %q", got)
	}
}

// clear(key) only wipes the visible notice if the keys match.
func TestNoticeBoardClearGuardsByKey(t *testing.T) {
	board, _ := newTestBoard()
	board.set(Notice{Key: "real", Text: "x", Kind: NoticeSticky})
	board.clear("stale")
	if got := board.currentText(); got != "x" {
		t.Errorf("stale clear must not wipe a different key, got %q", got)
	}
	board.clear("real")
	if got := board.currentText(); got != "" {
		t.Errorf("matching clear must wipe, got %q", got)
	}
}

// enqueue holds a flash while a turn is busy; flush releases it.
func TestNoticeBoardEnqueueThenFlush(t *testing.T) {
	board, _ := newTestBoard()
	board.enqueue(Notice{Key: "q", Text: "queued", Kind: NoticeFlash, TTLMS: 5000})
	if got := board.currentText(); got != "" {
		t.Errorf("expected nothing visible while busy, got %q", got)
	}
	board.flush()
	if got := board.currentText(); got != "queued" {
		t.Errorf("expected flush to surface queued notice, got %q", got)
	}
}
