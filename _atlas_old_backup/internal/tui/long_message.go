package tui

import "strings"

// LongMessageFolded is the rendered (collapsed, expanded) pair for a
// long user message. Mirrors Hermes's userDisplay: 300-char threshold,
// first-line + first-4-words label, max 80 chars, "[long message]"
// suffix. The expanded form is the full original text.
type LongMessageFolded struct {
	Collapsed string
	Expanded  string
	Threshold int
}

// IsLong reports whether the message exceeds the threshold. Atlas
// uses Hermes's 300-char cap.
func (l LongMessageFolded) IsLong() bool {
	return l.Threshold > 0
}

// foldLongMessage takes a raw user message and returns its folded
// representation. When the message is short, collapsed == expanded ==
// the original.
func foldLongMessage(text string) LongMessageFolded {
	if len(text) <= longMessageCharLimit {
		return LongMessageFolded{Collapsed: text, Expanded: text, Threshold: 0}
	}
	firstLine := text
	if nl := strings.Index(text, "\n"); nl >= 0 {
		firstLine = text[:nl]
	}
	words := strings.Fields(firstLine)
	head := strings.Join(words, " ")
	if len(head) > longMessageLabelMax {
		head = head[:longMessageLabelMax-1] + "…"
	}
	collapsed := head + " [long message]"
	return LongMessageFolded{Collapsed: collapsed, Expanded: text, Threshold: len(text)}
}
