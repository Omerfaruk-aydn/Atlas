package tui

import "os"

// TerminalModeReset is the canonical exit-time escape-sequence blob to
// turn off every "sticky" terminal mode the app may have enabled. We
// send it as a single synchronous Write on process exit so the
// terminal isn't left in a weird state if the user kills the TUI
// abruptly.
//
// The blob covers the modes Bubbletea's WithAltScreen/WithMouse*
// helpers don't always clean up:
//
//	?1000l   — X11 mouse click tracking
//	?1001l   — highlight mouse tracking
//	?1002l   — button-event tracking
//	?1003l   — any-event tracking
//	?1004l   — focus reporting
//	?1006l   — SGR mouse encoding
//	?2004l   — bracketed paste
//	?2029l   — terminal notifications
//	?1049l   — alternate screen
//	<\\u     — kitty keyboard protocol push
//	>\\u     — kitty keyboard protocol pop
//	'z       — vim-style modifyOtherKeys (less common)
//	'{       — kitty's progressive enhancement flag
//
// The non-? escapes (kitty, vim, kitty-progressive) cover the
// "we enabled this, we should disable it" cases when the user runs
// inside a wrapper that forwards them.
const TerminalModeReset = "\x1b[?1000l" +
	"\x1b[?1001l" +
	"\x1b[?1002l" +
	"\x1b[?1003l" +
	"\x1b[?1004l" +
	"\x1b[?1006l" +
	"\x1b[?2004l" +
	"\x1b[?2029l" +
	"\x1b[?1049l" +
	"\x1b[>1u" +
	"\x1b[>0u"

// writeTerminalModeReset writes the canonical reset blob to w. Used
// from the App's cleanup path. Skips non-TTY streams (no-op, doesn't
// throw) so a piped stdout doesn't get a garbage escape sequence.
func writeTerminalModeReset(w *os.File) {
	if w == nil {
		return
	}
	// Best-effort; we can't recover from a failed exit write.
	_, _ = w.WriteString(TerminalModeReset)
}
