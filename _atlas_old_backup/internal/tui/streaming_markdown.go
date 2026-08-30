package tui

import (
	"regexp"
	"strings"
)

// streamingMarkdownState is the incremental markdown scanner. The
// Hermes-flavoured optimization: split a growing markdown stream into
// "settled" (immutable, already-rendered) blocks plus a live "tail,"
// splitting only at a blank-line boundary that is not inside an open
// fenced code block or open `$$`/`\[` display-math block. The renderer
// can commit each settled block and re-render only the tail as it
// grows, so a long reply that finalizes into 200 blocks doesn't
// re-render all 200 on every token.
//
// The Atlas port uses a stateful scanner that accepts one chunk at a
// time and returns (settled, tail) so the caller can render settled
// via Glamour (cached) and the tail via the live renderer.
type streamingMarkdownState struct {
	// blocks is the list of settled (committed) blocks. Append-only.
	blocks []string
	// tail is the live, not-yet-settled buffer.
	tail string
	// inFence tracks whether the cursor is inside a ``` fenced block.
	inFence bool
	// fenceMarker records the opening ``` length (3 or 4+ backticks)
	// so the closer can be matched.
	fenceMarker string
	// inMath tracks whether the cursor is inside a $$..$$ / \[..\]
	// display-math block.
	inMath bool
	// mathMarker records the opening token ($$ or \[) so the closer
	// can be matched.
	mathMarker string
}

var (
	fenceRe   = regexp.MustCompile("^```")
	mathRe    = regexp.MustCompile("^\\$\\$|^\\\\\\[")
	mdBlankLn = regexp.MustCompile(`\n\s*\n`)
)

// NewStreamingMarkdownState returns an empty scanner.
func NewStreamingMarkdownState() *streamingMarkdownState {
	return &streamingMarkdownState{}
}

// Push appends a new chunk of markdown text. Returns (settled, tail)
// — settled is the list of newly-committed block strings (the
// caller may render these and cache the result); tail is the live
// buffer that should be re-rendered on every push.
func (s *streamingMarkdownState) Push(chunk string) (settled []string, tail string) {
	s.tail += chunk
	// Walk the tail scanning for blank-line boundaries. At each
	// boundary, if we're not inside a fence or math block, the
	// prefix is "settled" and we slice it off the tail.
	for {
		// Find the next blank-line boundary.
		idx := mdBlankLn.FindStringIndex(s.tail)
		if idx == nil {
			break
		}
		candidate := s.tail[:idx[0]]
		// Decide whether the candidate is "settled" or "live":
		// settled if the boundary isn't inside an open fence/math.
		if !s.containsOpenFenceOrMath(candidate) {
			settled = append(settled, candidate)
			s.blocks = append(s.blocks, candidate)
			s.tail = s.tail[idx[1]:]
			// Update in-fence/in-math state to reflect the
			// advancement through the candidate.
			s.advanceState(candidate)
			continue
		}
		// Inside an open block — wait for the closer before
		// considering the boundary final.
		break
	}
	s.advanceState("")
	// Setext heading protection: a "Title\n====\n" must be kept
	// contiguous with its paragraph (the underline is decoration,
	// not a separator). We achieve this by recognizing a setext
	// pattern just after a settled block and merging it back.
	settled = s.mergeSetextHeadings(settled)
	return settled, s.tail
}

// Final flushes whatever remains in the tail as a final settled
// block and resets the scanner. Returns the merged list of all
// blocks (previously settled + the tail).
func (s *streamingMarkdownState) Final() (settled []string) {
	settled = append([]string{}, s.blocks...)
	if s.tail != "" {
		settled = append(settled, s.tail)
	}
	s.blocks = nil
	s.tail = ""
	s.inFence = false
	s.inMath = false
	return settled
}

// containsOpenFenceOrMath reports whether settling the candidate
// would leave the scanner in an "open fence/math" state. A line that
// starts with "```" toggles the fence state (open → close → open);
// same for the math delimiters $$ and \[. The decision is:
// track depth from the start of the candidate; if depth>0 at the
// end, the candidate is unsafe to settle.
func (s *streamingMarkdownState) containsOpenFenceOrMath(candidate string) bool {
	fenceDepth := 0
	mathDepth := 0
	for _, line := range strings.Split(candidate, "\n") {
		trimmed := strings.TrimSpace(line)
		if fenceRe.MatchString(trimmed) {
			if fenceDepth > 0 {
				fenceDepth-- // closer
			} else {
				fenceDepth++ // opener
			}
		}
		if mathRe.MatchString(trimmed) {
			if mathDepth > 0 {
				mathDepth--
			} else {
				mathDepth++
			}
		}
	}
	return fenceDepth > 0 || mathDepth > 0
}

// advanceState updates inFence/inMath after a settled block has been
// sliced off. Walks the (now-removed) candidate once more to track
// fence/math boundaries.
func (s *streamingMarkdownState) advanceState(candidate string) {
	lines := strings.Split(candidate, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fenceRe.MatchString(trimmed) {
			if s.fenceMarker == "" {
				s.fenceMarker = trimmed
				s.inFence = true
			} else if trimmed == s.fenceMarker {
				s.fenceMarker = ""
				s.inFence = false
			}
		}
		if mathRe.MatchString(trimmed) {
			if s.mathMarker == "" {
				s.mathMarker = trimmed
				s.inMath = true
			} else if trimmed == s.mathMarker {
				s.mathMarker = ""
				s.inMath = false
			}
		}
	}
}

// mergeSetextHeadings scans the recently-settled blocks for any whose
// next neighbor is a setext heading underline (`====` or `----`)
// and merges them back into one block — the underline is decoration
// that must not be torn from its title.
func (s *streamingMarkdownState) mergeSetextHeadings(settled []string) []string {
	if len(settled) < 2 {
		return settled
	}
	merged := []string{settled[0]}
	for i := 1; i < len(settled); i++ {
		prev := merged[len(merged)-1]
		curr := settled[i]
		if isSetextUnderline(curr) && !strings.HasSuffix(prev, "\n") {
			merged[len(merged)-1] = prev + "\n" + curr
		} else {
			merged = append(merged, curr)
		}
	}
	return merged
}

// isSetextUnderline returns true if the (trimmed) string is a setext
// heading underline: a run of "=" or "-" characters, possibly with
// trailing spaces.
func isSetextUnderline(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	allEq := true
	allDash := true
	for _, r := range s {
		if r != '=' {
			allEq = false
		}
		if r != '-' {
			allDash = false
		}
	}
	return allEq || allDash
}
