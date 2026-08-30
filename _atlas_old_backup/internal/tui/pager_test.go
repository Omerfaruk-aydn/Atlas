package tui

import "testing"

func TestHandlePagerKeyScrollDown(t *testing.T) {
	p := PagerState{Title: "t", Content: "a\nb\nc\nd\ne", Offset: 0}
	next, res := handlePagerKey(p, "down")
	if res != PagerContinue || next.Offset != 1 {
		t.Errorf("down: expected offset 1 continue, got offset %d res %d", next.Offset, res)
	}
}

func TestHandlePagerKeyScrollUp(t *testing.T) {
	p := PagerState{Content: "a\nb\nc", Offset: 2}
	next, res := handlePagerKey(p, "up")
	if res != PagerContinue || next.Offset != 1 {
		t.Errorf("up: expected offset 1 continue, got offset %d res %d", next.Offset, res)
	}
}

func TestHandlePagerKeyClampsAtEdges(t *testing.T) {
	p := PagerState{Content: "a\nb", Offset: 0}
	_, res := handlePagerKey(p, "up")
	if res != PagerContinue {
		t.Errorf("up at top should continue, not close, got %d", res)
	}
	p2 := PagerState{Content: "a\nb", Offset: 1}
	_, res = handlePagerKey(p2, "down")
	if res != PagerContinue {
		t.Errorf("down at bottom should continue, got %d", res)
	}
}

func TestHandlePagerKeyPageDown(t *testing.T) {
	lines := ""
	for i := 0; i < 20; i++ {
		if i > 0 {
			lines += "\n"
		}
		lines += "line"
	}
	p := PagerState{Content: lines, Offset: 0}
	next, _ := handlePagerKey(p, "pagedown")
	if next.Offset <= 0 {
		t.Errorf("pagedown at 0 should advance, got %d", next.Offset)
	}
}

func TestHandlePagerKeyClose(t *testing.T) {
	p := PagerState{Content: "x"}
	for _, k := range []string{"q", "esc", "Q"} {
		_, res := handlePagerKey(p, k)
		if res != PagerClose {
			t.Errorf("%q should close, got %d", k, res)
		}
	}
}

func TestPagerHint(t *testing.T) {
	cases := []struct {
		off, end, total int
		mustContain    string
	}{
		{0, 5, 5, "kapat"},   // at top and bottom
		{0, 5, 100, "↓/j"},   // at top
		{50, 100, 100, "son"}, // at bottom
		{20, 60, 100, "sayfa"}, // middle
	}
	for _, c := range cases {
		hint := pagerHint(c.off, c.end, c.total)
		if !contains(hint, c.mustContain) {
			t.Errorf("pagerHint(%d,%d,%d) = %q, must contain %q", c.off, c.end, c.total, hint, c.mustContain)
		}
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
