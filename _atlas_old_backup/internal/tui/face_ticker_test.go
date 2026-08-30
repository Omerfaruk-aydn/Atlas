package tui

import (
	"strings"
	"testing"
	"time"
)

// All four face styles render something at any tick.
func TestFaceTickerAllStyles(t *testing.T) {
	now := time.Now()
	started := now.Add(-3 * time.Second)
	for _, style := range []FaceStyle{FaceStyleKaomoji, FaceStyleEmoji, FaceStyleAscii, FaceStyleUnicode} {
		view := faceTickerView{style: style, tick: 5, startedAt: started, now: now}.View()
		if view == "" {
			t.Errorf("style %d produced empty view", style)
		}
		if !strings.Contains(view, "3s") {
			t.Errorf("style %d missing elapsed clock, got %q", style, view)
		}
	}
}

// padVerb right-pads the verb and ends with the ellipsis so the status
// bar doesn't jitter as the verb rotates. Hermes's exact contract:
// every padded form must end with the ellipsis and must START with the
// verb text itself.
func TestPadVerbGluedEllipsis(t *testing.T) {
	padded := padVerb("düşünüyor")
	if !strings.HasPrefix(padded, "düşünüyor") {
		t.Errorf("padded form should preserve the verb as the prefix, got %q", padded)
	}
	if !strings.HasSuffix(padded, "…") {
		t.Errorf("padded form must end with the ellipsis, got %q", padded)
	}
	// All padded forms must share the same byte width — that's the
	// jitter guarantee.
	w := len(padded)
	for _, v := range thinkingVerbs {
		pv := padVerb(v)
		if !strings.HasPrefix(pv, v) {
			t.Errorf("padded form %q for verb %q does not start with the verb", pv, v)
		}
		if !strings.HasSuffix(pv, "…") {
			t.Errorf("padded form %q for verb %q must end with …", pv, v)
		}
		if len(pv) != w {
			t.Errorf("verb %q pads to %d, expected %d (all verbs should share width)", v, len(pv), w)
		}
	}
}

// faceTickIntervalForStyle returns sane positive durations.
func TestFaceTickIntervalForStyle(t *testing.T) {
	for _, s := range []FaceStyle{FaceStyleKaomoji, FaceStyleEmoji, FaceStyleAscii, FaceStyleUnicode} {
		d := faceTickIntervalForStyle(s)
		if d <= 0 {
			t.Errorf("style %d returned non-positive interval %v", s, d)
		}
	}
}
