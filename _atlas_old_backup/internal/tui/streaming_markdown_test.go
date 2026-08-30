package tui

import "testing"

func TestStreamingMarkdownSplitsOnBlankLine(t *testing.T) {
	s := NewStreamingMarkdownState()
	settled, tail := s.Push("hello world\n\nfoo")
	if len(settled) != 1 || settled[0] != "hello world" {
		t.Errorf("expected 1 settled block 'hello world', got %v", settled)
	}
	if tail != "foo" {
		t.Errorf("expected tail 'foo', got %q", tail)
	}
}

func TestStreamingMarkdownKeepsFenceTogether(t *testing.T) {
	s := NewStreamingMarkdownState()
	// A complete fenced block followed by a blank line and "after".
	// The scanner should settle the CLOSED fence block; "after" then
	// waits in the tail for its own blank-line boundary.
	chunk := "```go\nfunc main() {}\n```\n\nafter"
	settled, tail := s.Push(chunk)
	if len(settled) != 1 {
		t.Errorf("expected 1 settled block (the closed fence), got %d (%v)", len(settled), settled)
	}
	if len(settled) >= 1 && settled[0] != "```go\nfunc main() {}\n```" {
		t.Errorf("expected settled fence block, got %q", settled[0])
	}
	if tail != "after" {
		t.Errorf("expected tail 'after' (waiting for next blank line), got %q", tail)
	}
}

func TestStreamingMarkdownFinalFlushesTail(t *testing.T) {
	s := NewStreamingMarkdownState()
	_, _ = s.Push("para 1\n\npara 2 starts")
	final := s.Final()
	// Final() should return ALL accumulated blocks: the 1 settled
	// during Push() plus the 1 remaining in the tail.
	if len(final) != 2 {
		t.Errorf("expected 2 final blocks, got %d (%v)", len(final), final)
	}
}

func TestIsSetextUnderline(t *testing.T) {
	if !isSetextUnderline("===") {
		t.Error("=== should be a setext underline")
	}
	if !isSetextUnderline("----") {
		t.Error("---- should be a setext underline")
	}
	if isSetextUnderline("not") {
		t.Error("'not' should not be a setext underline")
	}
}
