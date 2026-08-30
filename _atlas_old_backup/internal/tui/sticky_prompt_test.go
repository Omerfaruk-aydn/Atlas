package tui

import "testing"

// stickyPromptLabel returns nothing when not streaming.
func TestStickyPromptLabelHiddenWhenIdle(t *testing.T) {
	app := newTestApp(80, 24)
	if got := app.stickyPromptLabel(); got != "" {
		t.Errorf("expected no sticky label when idle, got %q", got)
	}
}

// stickyPromptLabel surfaces a single-line collapsed version of the
// latest user message once other rows have accumulated below it.
func TestStickyPromptLabelShowsWhenScrolledAway(t *testing.T) {
	app := newTestApp(80, 24)
	app.streaming = true
	app.messages = []chatMessage{
		{role: "user", text: "bu uzun bir kullanıcı mesajıdır"},
		{role: "assistant", text: "kısmi cevap"},
		{role: "trail", text: "tool çalıştı"},
	}
	if got := app.stickyPromptLabel(); got == "" {
		t.Error("expected a sticky label when scrolled past the user message")
	}
}

// collapseForSticky trims long messages to a single line.
func TestCollapseForStickyTruncates(t *testing.T) {
	long := "this is a long user message that exceeds the sticky label's character budget, in fact it is much much longer than the cap"
	got := collapseForSticky(long)
	if len(got) > stickyMaxLen {
		t.Errorf("collapseForSticky must cap at %d chars, got %d", stickyMaxLen, len(got))
	}
	if got[len(got)-len("…"):] != "…" {
		t.Errorf("collapsed sticky should end with …, got %q", got)
	}
}

// collapseForSticky collapses internal whitespace.
func TestCollapseForStickyCollapsesWhitespace(t *testing.T) {
	got := collapseForSticky("a\n\t  b   c")
	if got != "a b c" {
		t.Errorf("expected whitespace-collapsed form, got %q", got)
	}
}
