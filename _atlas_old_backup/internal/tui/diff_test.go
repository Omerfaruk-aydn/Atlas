package tui

import (
	"strings"
	"testing"
)

func TestRenderDiffShowsAddedAndRemovedLines(t *testing.T) {
	a := &App{theme: DefaultTheme()}

	old := "line1\nline2\nline3"
	new_ := "line1\nCHANGED\nline3"

	out := a.renderDiff(old, new_)

	if !strings.Contains(out, "- line2") {
		t.Errorf("expected removed line marker for line2, got:\n%s", out)
	}
	if !strings.Contains(out, "+ CHANGED") {
		t.Errorf("expected added line marker for CHANGED, got:\n%s", out)
	}
}

func TestRenderDiffNewFileHasNoRemovals(t *testing.T) {
	a := &App{theme: DefaultTheme()}

	out := a.renderDiff("", "brand new content")

	if strings.Contains(out, "- ") {
		t.Errorf("expected no removed lines for a brand new file, got:\n%s", out)
	}
	if !strings.Contains(out, "+ brand new content") {
		t.Errorf("expected the whole new content marked as added, got:\n%s", out)
	}
}

func TestRenderDiffCollapsesLongUnchangedContext(t *testing.T) {
	a := &App{theme: DefaultTheme()}

	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "context"
	}
	old := strings.Join(lines, "\n")
	new_ := strings.Replace(old, "context\ncontext\ncontext\ncontext\ncontext\ncontext\ncontext\ncontext\ncontext\ncontext",
		"context\ncontext\ncontext\ncontext\ncontext\nCHANGED\ncontext\ncontext\ncontext\ncontext", 1)

	out := a.renderDiff(old, new_)

	if !strings.Contains(out, "satır değişmedi") {
		t.Errorf("expected long unchanged runs to collapse with a summary marker, got:\n%s", out)
	}
}
