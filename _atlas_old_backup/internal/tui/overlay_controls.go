package tui

// OverlayKeys is the standard keymap for dismissable overlays. Every
// overlay in Hermes (skills hub, plugins hub, model picker, sessions,
// etc.) routes through this — the routing itself happens in the
// per-overlay keymap, but the dismiss/back/close semantics are
// centralized here. Mirrors Hermes's useOverlayKeys hook.
type OverlayKeys struct {
	OnBack  func() // q-style "back" action (closes the overlay)
	OnClose func() // Esc-style "close" action (also closes, but lower-priority)
}

// handleOverlayKey dispatches one key through the standard overlay
// keymap. Returns true if the key was consumed (and the closure fired).
// Keeps the App's Update loop uniform: every overlay checks its own
// keymap first, then falls back to handleOverlayKey for the universal
// Esc / q semantics.
func (k OverlayKeys) Handle(key string) bool {
	switch key {
	case "esc":
		if k.OnBack != nil {
			k.OnBack()
			return true
		}
		if k.OnClose != nil {
			k.OnClose()
			return true
		}
		return false
	case "q":
		if k.OnClose != nil {
			k.OnClose()
			return true
		}
		return false
	}
	return false
}

// windowOffset is the canonical "keep selection visible in an N-row
// viewport, centered-ish, clamped to bounds" utility used by every
// list overlay in Hermes (agents Gantt, session switcher, model picker,
// plugins hub, skills hub). Atlas's port takes the current selection
// and viewport size, returns the offset of the top visible row.
func windowOffset(sel, viewportSize, total int) int {
	if total <= viewportSize {
		return 0
	}
	half := viewportSize / 2
	off := sel - half
	if off < 0 {
		off = 0
	}
	if off > total-viewportSize {
		off = total - viewportSize
	}
	return off
}

// stepRow and snapRow implement the journey.tsx "cursor never lands on
// a blank spacer row" pattern: given the current cursor and a predicate
// that says whether a row is "blank", stepRow advances in `delta`
// direction skipping blank rows, snapRow jumps to the first non-blank
// row. Both wrap around. Useful for any list with section headers.
func stepRow(cur, delta, n int, isBlank func(int) bool) int {
	if n <= 0 {
		return 0
	}
	cur = (cur + delta + n) % n
	for isBlank(cur) {
		cur = (cur + delta + n) % n
		// Safety: if EVERY row is blank, just return where we started.
		// (Shouldn't happen but guards against infinite loop.)
		if cur == (cur+delta+n)%n {
			return cur
		}
	}
	return cur
}
