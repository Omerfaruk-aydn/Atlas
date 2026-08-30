package tui

import (
	"os"
)

// AltScreenEntry is the canonical escape sequence to enter the
// alternate screen buffer. Sent before the first render so the
// program's output replaces the shell scrollback entirely. The
// matching exit sequence lives in TerminalModeReset.
const AltScreenEntry = "\x1b[?1049h"

// AltScreenExit is the explicit alternate-screen exit (used when
// the user explicitly toggles off alt-screen via /alt-screen, or
// when a sub-process wants the shell back). TerminalModeReset also
// contains this.
const AltScreenExit = "\x1b[?1049l"

// enterAltScreen writes the alt-screen entry to w. Idempotent on
// already-entered alt-screens (the terminal handles double-entry as
// a no-op for ?1049h).
func enterAltScreen(w *os.File) {
	if w == nil {
		return
	}
	_, _ = w.WriteString(AltScreenEntry)
}

// exitAltScreen writes the alt-screen exit to w. Always paired with
// enterAltScreen — the program only invokes it on quit.
func exitAltScreen(w *os.File) {
	if w == nil {
		return
	}
	_, _ = w.WriteString(AltScreenExit)
}

// BracketedPasteEnable / BracketedPasteDisable toggle the
// CSI ?2004h mode. Bubbletea enables this by default; the App
// explicitly sends the entry on Init and the disable on quit for
// the cases where Bubbletea's lifecycle is bypassed (e.g. when the
// user runs Atlas in a non-Bubbletea shell wrapper).
const BracketedPasteEnable = "\x1b[?2004h"
const BracketedPasteDisable = "\x1b[?2004l"

// DECSET 1004 (focus reporting) — CSI ?1004h / ?1004l. When enabled,
// the terminal emits CSI I / CSI O on focus changes. The App listens
// for these in the input parser and updates uiStore.terminalFocused.
const FocusReportingEnable = "\x1b[?1004h"
const FocusReportingDisable = "\x1b[?1004l"

// Kitty keyboard protocol push/pop. Sent as part of the alt-screen
// entry/exit so Atlas can use the protocol when the terminal
// supports it (without the protocol, Cmd vs Alt can't be told apart).
const KittyKbdEnable = "\x1b[>1u"
const KittyKbdDisable = "\x1b[>0u"

// SGR mouse modes (1006 = SGR encoding; 1000 = X11 click tracking;
// 1002 = button-event tracking; 1003 = any-event). Atlas leaves
// these disabled — no mouse support today — but exposes the
// constants for future use.
const MouseSGREnable = "\x1b[?1006h"
const MouseSGRDisable = "\x1b[?1006l"
const MouseClickEnable = "\x1b[?1000h"
const MouseClickDisable = "\x1b[?1000l"
const MouseButtonEnable = "\x1b[?1002h"
const MouseButtonDisable = "\x1b[?1002l"
const MouseAnyEnable = "\x1b[?1003h"
const MouseAnyDisable = "\x1b[?1003l"

// enterTerminalCaps writes the alt-screen + bracketed-paste + focus-
// reporting + kitty-kbd entry sequence. Called from the App's Init
// or from any sub-process that wants the full TUI capability set.
func enterTerminalCaps(w *os.File) {
	if w == nil {
		return
	}
	_, _ = w.WriteString(AltScreenEntry)
	_, _ = w.WriteString(BracketedPasteEnable)
	_, _ = w.WriteString(FocusReportingEnable)
	_, _ = w.WriteString(KittyKbdEnable)
}

// exitTerminalCaps writes the canonical exit sequence. Always called
// on Quit (and on panic-recovery defer paths).
func exitTerminalCaps(w *os.File) {
	if w == nil {
		return
	}
	_, _ = w.WriteString(TerminalModeReset)
}
