package tui

import "time"

// NoticeKind categorizes a transient status-bar overlay. The two
// categories differ in lifetime, mirroring Hermes's "Strategy B" notice
// system: a "flash" notice (e.g. "rate limited, retrying") is cleared
// automatically the instant a new turn starts; a "sticky" notice (e.g.
// "auth expired, run /login") persists across turns until something
// explicitly clears it.
type NoticeKind int

const (
	NoticeFlash NoticeKind = iota
	NoticeSticky
)

// NoticeLevel drives the color tier the status bar paints it in. Hermes
// uses error/warn/info/success; Atlas mirrors that vocabulary so the
// theme's existing tier colors (statusCritical/statusBad/statusGood/etc.)
// light up without any new derivation work.
type NoticeLevel int

const (
	NoticeLevelInfo NoticeLevel = iota
	NoticeLevelSuccess
	NoticeLevelWarn
	NoticeLevelError
)

// Notice is a single status-bar overlay. The Key field is a stable
// identifier the clearNotice call compares against — that way a stale
// clear (e.g. from a turn that was already replaced) can't accidentally
// wipe a newer notice that just took the slot.
type Notice struct {
	Key       string
	Text      string
	Level     NoticeLevel
	Kind      NoticeKind
	VisibleAt time.Time // set when the notice actually takes the visible slot
	TTLMS     int64     // 0 = no auto-expire (sticky-only); >0 = visible window in ms
}

// visible reports whether the notice is still within its TTL window.
// "Sticky" notices never auto-expire.
func (n Notice) visible(now time.Time) bool {
	if n.Kind == NoticeSticky {
		return true
	}
	if n.TTLMS <= 0 {
		return true
	}
	return now.Sub(n.VisibleAt) < time.Duration(n.TTLMS)*time.Millisecond
}

// NoticeBoard is the status-bar's overlay slot. Only one notice can be
// visible at a time (matching the Hermes model); a new flash notice
// displaces the previous flash but never a sticky one — sticky wins
// until explicitly cleared.
type NoticeBoard struct {
	current  *Notice
	pending  *Notice // held while a turn is busy; flushed at the 3 real turn-end sites
	clock    func() time.Time
}

// newNoticeBoard returns an empty board; clock is the wall-clock source
// (overridable in tests for deterministic TTL math).
func newNoticeBoard(clock func() time.Time) *NoticeBoard {
	if clock == nil {
		clock = time.Now
	}
	return &NoticeBoard{clock: clock}
}

// set replaces the currently-visible notice immediately, but only if the
// new notice is at least as sticky as the current one. A pending notice
// (held while a turn is busy) is NOT shown here — see enqueue for that
// path.
func (b *NoticeBoard) set(n Notice) {
	if n.Kind == NoticeFlash && b.current != nil && b.current.Kind == NoticeSticky {
		// A flash never displaces a sticky — the sticky has to be
		// explicitly cleared before the flash can take the slot.
		return
	}
	n.VisibleAt = b.clock()
	if n.VisibleAt.IsZero() {
		n.VisibleAt = time.Now()
	}
	b.current = &n
}

// enqueue buffers a notice while a turn is busy. The FaceTicker owns the
// visible slot during a turn; we hold the notice until turn end. flush
// drains any held notice into the visible slot.
func (b *NoticeBoard) enqueue(n Notice) {
	if n.Kind == NoticeSticky {
		// Sticky notices ignore the busy guard — they're important
		// enough to interrupt the spinner for.
		b.set(n)
		return
	}
	b.pending = &n
}

// flush moves any held notice into the visible slot. Called at the three
// real turn-end sites (message complete, interrupt, error) — NEVER
// inside resetTurn (which also runs on session switch and would leak the
// notice across sessions).
func (b *NoticeBoard) flush() {
	if b.pending == nil {
		return
	}
	b.set(*b.pending)
	b.pending = nil
}

// clear removes a notice by key. If the visible notice's Key doesn't
// match, the call is a no-op — protects against a stale clear wiping a
// newer notice.
func (b *NoticeBoard) clear(key string) {
	if b.current != nil && b.current.Key == key {
		b.current = nil
	}
}

// currentText returns the visible notice's text, or "" if none. Used by
// the status bar to decide whether to render the notice slot at all.
func (b *NoticeBoard) currentText() string {
	if b.current == nil {
		return ""
	}
	if !b.current.visible(b.clock()) {
		b.current = nil
		return ""
	}
	return b.current.Text
}
