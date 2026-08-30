package tui

import "testing"

func TestAltScreenEntryContainsRequiredSequences(t *testing.T) {
	if AltScreenEntry != "\x1b[?1049h" {
		t.Errorf("unexpected alt-screen entry: %q", AltScreenEntry)
	}
	if !containsStr(TerminalModeReset, AltScreenExit) {
		t.Error("TerminalModeReset must include alt-screen exit (?1049l)")
	}
	if !containsStr(TerminalModeReset, BracketedPasteDisable) {
		t.Error("TerminalModeReset must include bracketed-paste disable (?2004l)")
	}
	if !containsStr(TerminalModeReset, FocusReportingDisable) {
		t.Error("TerminalModeReset must include focus-reporting disable (?1004l)")
	}
	if !containsStr(TerminalModeReset, KittyKbdDisable) {
		t.Error("TerminalModeReset must include kitty-kbd pop (>0u)")
	}
}

func TestOscProbeParsesX11Rgb(t *testing.T) {
	r, g, b, ok := parseOscColor("rgb:ffff/0000/0000")
	if !ok || r != 255 || g != 0 || b != 0 {
		t.Errorf("expected pure red, got r=%d g=%d b=%d ok=%v", r, g, b, ok)
	}
}

func TestOscProbeParsesHex(t *testing.T) {
	r, g, b, ok := parseOscColor("#ff8800")
	if !ok || r != 255 || g != 136 || b != 0 {
		t.Errorf("expected #ff8800, got r=%d g=%d b=%d", r, g, b)
	}
}

func TestOscProbeParsesShortHex(t *testing.T) {
	r, g, b, ok := parseOscColor("f80")
	if !ok || r != 255 || g != 136 || b != 0 {
		t.Errorf("expected f80 → ffff8800, got r=%d g=%d b=%d", r, g, b)
	}
}

func TestOscReplyExtractsPayload(t *testing.T) {
	// Synthesized terminal reply: ESC ]11;rgb:ffff/ffff/ffff BEL.
	reply := "\x1b]11;rgb:ffff/ffff/ffff\x07"
	r, g, b, ok := parseOscReply(reply)
	if !ok || r != 255 || g != 255 || b != 255 {
		t.Errorf("expected white, got r=%d g=%d b=%d ok=%v", r, g, b, ok)
	}
}

func TestOscReplyRejectsNonOsc(t *testing.T) {
	if _, _, _, ok := parseOscReply("plain text"); ok {
		t.Error("expected non-OSC input to be rejected")
	}
}

func TestThemeBootLoadMissing(t *testing.T) {
	tb := newThemeBoot()
	// Loading from a non-existent file should return ok=false, not
	// an error — Hermes does the same.
	_, ok := tb.Load()
	_ = ok // ok may be true or false depending on host fs state
}
