package tui

import (
	"strings"
	"testing"
	"time"
)

// stripAnsi removes all CSI/OSC sequences, leaves plain text.
func TestStripAnsiRemovesSequences(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]0;title\x07after", "after"},
		{"a\x1b[1mb\x1b[0mc", "abc"},
	}
	for _, c := range cases {
		if got := stripAnsi(c.in); got != c.want {
			t.Errorf("stripAnsi(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// sanitizeAnsiForRender keeps SGR, drops everything else.
func TestSanitizeAnsiKeepsSGR(t *testing.T) {
	in := "\x1b[31mred\x1b[0m\x1b[2J"
	got := sanitizeAnsiForRender(in)
	if !strings.Contains(got, "\x1b[31m") {
		t.Error("sanitizeAnsiForRender should keep SGR color codes")
	}
	if strings.Contains(got, "\x1b[2J") {
		t.Error("sanitizeAnsiForRender should strip clear-screen")
	}
}

// cleanThinkingText strips the "Thinking..." prefix.
func TestCleanThinkingTextStripsPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Thinking... we need to check the API.", "we need to check the API."},
		{"Let me think about this. First, ...", "First, ..."},
		{"normal reply without thinking prefix", "normal reply without thinking prefix"},
		{"", ""},
	}
	for _, c := range cases {
		if got := cleanThinkingText(c.in); got != c.want {
			t.Errorf("cleanThinkingText(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// boundedLiveRenderText adds an "[omitted]" header when truncating.
func TestBoundedLiveRenderTextTruncates(t *testing.T) {
	body := strings.Repeat("line\n", 100)
	got := boundedLiveRenderText(body, 80, 5)
	if !strings.HasPrefix(got, "[atlandı:") {
		t.Errorf("expected omitted header, got %q", got[:30])
	}
}

// boundedLiveRenderText returns the input unchanged when it fits.
func TestBoundedLiveRenderTextNoTruncate(t *testing.T) {
	body := "short\ntext"
	if got := boundedLiveRenderText(body, 1000, 100); got != body {
		t.Errorf("expected unchanged, got %q", got)
	}
}

// fmtDuration cases match Hermes's tiered formatter.
func TestFmtDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m 30s"},
		{3600 * time.Second, "1h 0m"},
		{3725 * time.Second, "1h 2m"},
	}
	for _, c := range cases {
		if got := fmtDuration(c.in); got != c.want {
			t.Errorf("fmtDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fmtK renders thousands compactly and elides zero.
func TestFmtK(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, ""},
		{1, "1"},
		{542, "542"},
		{1234, "1.2k"},
		{12345, "12k"},
		{100000, "100k"},
	}
	for _, c := range cases {
		if got := fmtK(c.in); got != c.want {
			t.Errorf("fmtK(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
