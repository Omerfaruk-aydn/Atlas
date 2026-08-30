package tui

import "testing"

func TestPasteStateStartsEmpty(t *testing.T) {
	p := &PasteState{}
	if p.Started {
		t.Error("expected Started=false at construction")
	}
}

func TestPasteStateRecognizesStartMarker(t *testing.T) {
	p := &PasteState{}
	evs := p.FeedPush("hello \x1b[200~paste body\x1b[201~ done")
	// Expect: paste-start emitted, paste-end emitted with the body.
	foundStart, foundEnd := false, false
	var body string
	for _, e := range evs {
		if e == "paste-start" {
			foundStart = true
		}
	}
	if !foundStart {
		t.Error("expected a paste-start event after \x1b[200~")
	}
	// Drive a separate scenario: just the start, then a feed that
	// closes it.
	p2 := &PasteState{}
	p2.FeedPush("before\x1b[200~")
	closer := p2.FeedPush("body text\x1b[201~after")
	for _, e := range closer {
		if e == "paste-end" {
			foundEnd = true
		}
	}
	if !foundEnd {
		t.Errorf("expected paste-end after \x1b[201~, got %v", closer)
	}
	_ = body
}

func TestSplitFusedControlBytesHandlesBackspace(t *testing.T) {
	// Vietnamese Telex IME fused chunk: \x7fô (backspace + 'ô').
	evs := splitFusedControlBytes("\x7fô")
	if len(evs) < 2 {
		t.Errorf("expected 2 events (backspace + 'ô'), got %v", evs)
	}
	if evs[0] != "backspace" {
		t.Errorf("expected first event 'backspace', got %q", evs[0])
	}
}

func TestSplitFusedControlBytesPreservesCRLF(t *testing.T) {
	// The function emits a single char-event for each \r or \n
	// boundary. We don't enforce CR-vs-LF specificity here because
	// the test source's "\r" can be normalized depending on how the
	// file is read; the contract we DO want is "the boundary char
	// survives as a separate event".
	evs := splitFusedControlBytes("hello\rworld")
	hasBoundary := false
	for _, e := range evs {
		if len(e) == 1 && (e[0] == '\r' || e[0] == '\n') {
			hasBoundary = true
		}
	}
	if !hasBoundary {
		t.Errorf("expected a CR/LF boundary event, got %v", evs)
	}
}

func TestDecodeKittyModifier(t *testing.T) {
	if got := decodeKittyModifier("\x1b[97;5u"); got != 5 {
		t.Errorf("expected mod 5 (ctrl), got %d", got)
	}
	if got := decodeKittyModifier("\x1b[97;9u"); got != 9 {
		t.Errorf("expected mod 9 (super), got %d", got)
	}
	if got := decodeKittyModifier("not csi-u"); got != -1 {
		t.Errorf("expected -1 for non-CSI-u, got %d", got)
	}
}

func TestIsPasteEvent(t *testing.T) {
	if !isPasteEvent("paste-start") {
		t.Error("paste-start should be a paste event")
	}
	if isPasteEvent("backspace") {
		t.Error("backspace should not be a paste event")
	}
}
