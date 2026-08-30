package tui

import "testing"

// splitComposerHighlights reconstructs the input exactly.
func TestSplitComposerHighlightsRoundTrip(t *testing.T) {
	cases := []string{
		"",
		"hello world",
		"/model gpt-4",
		"@file:foo.go",
		"@file:`my notes.md`",
		"with @file:a.go and /help",
		"@file:foo [[ Image 1 ]] more text",
	}
	for _, in := range cases {
		segs := splitComposerHighlights(in)
		var got string
		for _, s := range segs {
			got += s.Text
		}
		if got != in {
			t.Errorf("round trip failed for %q: got %q", in, got)
		}
	}
}

// Highlighted segments carry the right kinds.
func TestSplitComposerHighlightsKinds(t *testing.T) {
	segs := splitComposerHighlights("/help me @file:foo.go")
	var kinds []highlightKind
	for _, s := range segs {
		if s.Kind == HighlightPlain {
			continue
		}
		kinds = append(kinds, s.Kind)
	}
	if len(kinds) < 2 {
		t.Fatalf("expected at least 2 highlighted segments, got %d (%v)", len(kinds), kinds)
	}
	if kinds[0] != HighlightSlash {
		t.Errorf("expected first highlight to be HighlightSlash, got %d", kinds[0])
	}
}

// Absolute paths must NOT be highlighted as slash commands.
func TestSplitComposerHighlightsIgnoresAbsolutePaths(t *testing.T) {
	segs := splitComposerHighlights("see /usr/local/bin")
	for _, s := range segs {
		if s.Kind == HighlightSlash {
			t.Errorf("absolute path /usr/local should not be highlighted, got %+v", s)
		}
	}
}

// A half-typed slash is still highlighted.
func TestSplitComposerHighlightsHalfTyped(t *testing.T) {
	segs := splitComposerHighlights("/wor")
	found := false
	for _, s := range segs {
		if s.Kind == HighlightSlash {
			found = true
		}
	}
	if !found {
		t.Error("a half-typed slash must still be highlighted")
	}
}

// highlightMask is per-character and tracks the kind faithfully.
func TestHighlightMaskMatchesKinds(t *testing.T) {
	segs := []highlightSegment{
		{Kind: HighlightPlain, Text: "ab"},
		{Kind: HighlightSlash, Text: "/x"},
	}
	mask := highlightMask(segs)
	if len(mask) != 4 {
		t.Fatalf("expected 4 chars in mask (2 plain + 2 slash), got %d", len(mask))
	}
	if mask[0] || mask[1] || !mask[2] || !mask[3] {
		t.Errorf("expected [false false true true], got %v", mask)
	}
}

// highlightsStable: if no character changes its highlight state, the
// fast-echo bypass can skip the repaint. /wor → /work keeps all prior
// cells in the same state.
func TestHighlightsStableTrueOnNoChange(t *testing.T) {
	a := highlightMask(splitComposerHighlights("/wor"))
	b := highlightMask(splitComposerHighlights("/work"))
	if !highlightsStable(a, b) {
		t.Error("extending /wor → /work keeps all prior cells in the same state — must be stable")
	}
}

// highlightsStable: typing more chars that FLIP a prior cell's state
// (here: "abc" → "/abc", where the slash is a new highlight) must
// return false so the renderer does a full repaint.
func TestHighlightsStableFalseOnRecolor(t *testing.T) {
	a := highlightMask(splitComposerHighlights("abc"))
	b := highlightMask(splitComposerHighlights("/abc"))
	if highlightsStable(a, b) {
		t.Error("adding a leading slash to 'abc' must flip the first cell's mask bit")
	}
}
