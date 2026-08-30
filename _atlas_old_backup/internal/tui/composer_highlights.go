package tui

import "regexp"

// highlightKind tags a slice of composer text with the semantic role
// that should drive its color. Non-highlighted text is HighlightPlain.
type highlightKind int

const (
	HighlightPlain highlightKind = iota
	HighlightSlash
	HighlightAtRef
	HighlightPaste
)

// highlightSegment is one (kind, text) pair returned by
// splitComposerHighlights. The concatenation of every Text field is
// always the original input — this invariant is critical so the
// composer never silently drops characters when the highlighter is on.
type highlightSegment struct {
	Kind highlightKind
	Text string
}

// tokenRegexes is the ordered list of token matchers. The highlighter
// walks left-to-right and the FIRST regex that matches at the current
// position wins — so a slash inside a backtick-quoted @ref value stays
// part of the ref, and a paste token wins over embedded slashes.
var tokenRegexes = []struct {
	Kind highlightKind
	Re   *regexp.Regexp
}{
	{HighlightPaste, regexp.MustCompile(`\[\[ [^\]]+ \]\]`)},
	// @ref: @file:, @diff, @staged, with optional backtick-quoted value
	// supporting spaces (e.g. @file:`my notes.md`).
	{HighlightAtRef, regexp.MustCompile(`@(file|diff|staged)(?::[^\s]+|:\x60[^\x60]+\x60)?`)},
	// Slash command: at the head of a token boundary (whitespace/start)
	// followed by an identifier. Go's regexp engine doesn't support
	// negative lookahead, so the post-match check in splitComposerHighlights
	// rejects a slash match whose next char is "/" — that's what would
	// distinguish a command ("/help me") from a path ("/usr/local").
	// This matches Hermes's composerHighlights.test.ts expectations:
	// "absolute paths ( /usr/local/bin ), relative paths
	// ( src/foo/bar ), bare math-looking slashes ( 3 /4 ), standalone /,
	// email addresses" are all explicitly NOT highlighted.
	{HighlightSlash, regexp.MustCompile(`(?:^|[\s\n])/[A-Za-z][\w-]*`)},
}

// splitComposerHighlights scans input left-to-right and emits a sequence
// of (kind, text) pairs. The total concatenation reconstructs the input
// exactly. Plain (non-matching) runs are emitted as HighlightPlain.
//
// The matching rule (transcribed from Hermes's composerHighlights.ts):
// the regex whose match starts at the EARLIEST position wins. Ties
// resolve by the tokenRegexes declaration order (paste > at_ref >
// slash), so a paste token that overlaps an @ref still wins.
func splitComposerHighlights(input string) []highlightSegment {
	if input == "" {
		return nil
	}
	var out []highlightSegment
	i := 0
	plain := func(s string) {
		if s == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].Kind == HighlightPlain {
			out[n-1].Text += s
		} else {
			out = append(out, highlightSegment{Kind: HighlightPlain, Text: s})
		}
	}
	for i < len(input) {
		// Find the earliest match across all regexes. Slash candidates
		// are validated post-match: if the very next char is a "/",
		// we're looking at a path like /usr/local, not a command, so
		// the candidate is rejected (treated as a non-match for this
		// iteration; the slash will fall through to plain and the
		// path's later tokens can still match).
		bestStart := -1
		bestEnd := -1
		bestKind := HighlightPlain
		for _, t := range tokenRegexes {
			loc := t.Re.FindStringIndex(input[i:])
			if loc == nil {
				continue
			}
			start := i + loc[0]
			end := i + loc[1]
			// Slash regex's leading "whitespace or start" anchor is a
			// zero-width match at the boundary; only accept it when the
			// anchor's effective start is the same as the slash's start
			// (i.e. we're at the start of input or the prior char was
			// whitespace).
			if t.Kind == HighlightSlash {
				if start == i && input[start] != '/' {
					// We're at start but didn't actually see a slash;
					// skip this candidate.
					continue
				}
				// Reject if the char after the match is another slash —
				// that's a path, not a command. We trim the match down
				// to exclude the leading "/", leaving the next-iteration
				// matcher to find anything inside the path.
				if end < len(input) && input[end] == '/' {
					continue
				}
			}
			if bestStart == -1 || start < bestStart || (start == bestStart && end > bestEnd) {
				bestStart = start
				bestEnd = end
				bestKind = t.Kind
			}
		}
		if bestStart == -1 {
			// No match from here to end — emit the rest as plain.
			plain(input[i:])
			break
		}
		if bestStart > i {
			plain(input[i:bestStart])
		}
		out = append(out, highlightSegment{Kind: bestKind, Text: input[bestStart:bestEnd]})
		i = bestEnd
	}
	return out
}

// highlightMask is a per-character boolean slice where true means the
// character at that position is in a highlighted (non-plain) segment.
// Useful for fast-echo bypass decisions: if a previously-rendered
// character's mask bit hasn't changed, the cursor-advance write can skip
// the styling pass.
func highlightMask(segs []highlightSegment) []bool {
	var mask []bool
	for _, s := range segs {
		for range s.Text {
			if s.Kind == HighlightPlain {
				mask = append(mask, false)
			} else {
				mask = append(mask, true)
			}
		}
	}
	return mask
}

// highlightsStable returns true if every character already on screen
// keeps the same highlight state across the re-tokenize. Used as a
// fast-echo bypass gate: a token that just *grew* (`/wor` → `/work`)
// keeps every prior cell in the same state, so the renderer can
// append-stamp the new cells rather than repaint the whole composer.
func highlightsStable(prevMask, newMask []bool) bool {
	n := len(prevMask)
	if n > len(newMask) {
		n = len(newMask)
	}
	for i := 0; i < n; i++ {
		if prevMask[i] != newMask[i] {
			return false
		}
	}
	return true
}
