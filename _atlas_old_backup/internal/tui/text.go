package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ansiCSI matches the entire CSI family (cursor/erase/SGR/etc.) so
// stripAnsi/sanitizeAnsiForRender can drop them. OSC and DCS are handled
// separately below. Conservative on purpose: the string slice fed in here
// may have been half-consumed at a buffer boundary, so the regex accepts
// the CSI introducer even with no parameters and a final byte outside the
// canonical 0x40-0x7E range — those malformed tails are then discarded.
var ansiCSI = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]?`)

// ansiOSC matches Operating System Command sequences (terminated by BEL or
// ST). The non-greedy match keeps a stray trailing ESC from swallowing the
// rest of the buffer.
var ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// ansiSGR matches the subset of CSI that is a color/style run, so we can
// keep it during sanitizeAnsiForRender (rendering still needs SGR) while
// stripping every other control sequence.
var ansiSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripAnsi removes all ANSI control sequences (CSI/OSC/DCS). Use when
// computing display width or feeding text into a non-rendering context.
func stripAnsi(s string) string {
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiCSI.ReplaceAllString(s, "")
	return s
}

// sanitizeAnsiForRender keeps SGR color/style runs (so styled text still
// renders correctly through Glamour/Lipgloss) but strips every other ANSI
// sequence — cursor movement, screen clear, OSC titles — that would
// otherwise clobber the terminal mid-render when fed untrusted tool output.
func sanitizeAnsiForRender(s string) string {
	s = ansiOSC.ReplaceAllString(s, "")
	// Replace every non-SGR CSI with empty.
	s = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]?`).
		ReplaceAllStringFunc(s, func(seq string) string {
			if ansiSGR.MatchString(seq) {
				return seq
			}
			return ""
		})
	return s
}

// thinkingVerbNoise is the set of status-verb phrases that streaming models
// sometimes emit as visible text at the head of their reasoning channel
// (e.g. "Thinking..."). cleanThinkingText strips them so the transcript
// doesn't double-render the busy-indicator verb in two places at once.
//
// Matched at the head of the string only — a model that genuinely says
// "let me think about this" mid-reply still gets to keep those words.
// The list is the longer phrases first so the regex engine prefers
// them; trailing class `[^\p{L}]*` consumes trailing spaces and
// punctuation.
var thinkingVerbNoise = regexp.MustCompile(`(?i)^\s*(let me think about this|let me think|thinking about this|thinking|reasoning|pondering|considering)[^\p{L}]*`)

// cleanThinkingText removes the "Thinking..." / "Let me think..." /
// "Reasoning..." prefixes that some providers leak into the visible stream
// while reasoning. Cheap optimization: a substring scan of the first 64
// chars skips the regex pass entirely on the common case.
func cleanThinkingText(s string) string {
	if s == "" {
		return s
	}
	head := s
	if len(head) > 64 {
		head = head[:64]
	}
	low := strings.ToLower(head)
	if !strings.Contains(low, "thinking") &&
		!strings.Contains(low, "reasoning") &&
		!strings.Contains(low, "let me think") &&
		!strings.Contains(low, "pondering") &&
		!strings.Contains(low, "considering") {
		return s
	}
	cleaned := thinkingVerbNoise.ReplaceAllString(s, "")
	return strings.TrimLeft(cleaned, " \t")
}

// boundedLiveRenderText caps a streaming buffer to at most maxChars / maxLines
// visible; when truncated, emits an [omitted N chars / N lines] header so
// the user sees *that* truncation happened rather than a silent cut-off.
// Walks backward from the tail to find a clean line boundary (no in-progress
// partial last line kept).
func boundedLiveRenderText(s string, maxChars, maxLines int) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	omittedLines := 0
	if len(lines) > maxLines {
		omittedLines = len(lines) - maxLines
		// Keep only the tail.
		lines = lines[len(lines)-maxLines:]
	}

	body := strings.Join(lines, "\n")
	omittedChars := 0
	if len(body) > maxChars {
		omittedChars = len(body) - maxChars
		body = body[len(body)-maxChars:]
		// Re-anchor to a line boundary so we don't start mid-word.
		if nl := strings.Index(body, "\n"); nl >= 0 && nl < len(body)-1 {
			body = body[nl+1:]
		}
	}
	if omittedLines == 0 && omittedChars == 0 {
		return s
	}
	var headerParts []string
	if omittedLines > 0 {
		headerParts = append(headerParts, fmt.Sprintf("%d satır", omittedLines))
	}
	if omittedChars > 0 {
		headerParts = append(headerParts, fmt.Sprintf("%d karakter", omittedChars))
	}
	return fmt.Sprintf("[atlandı: %s]\n%s", strings.Join(headerParts, " / "), body)
}

// fmtDuration renders a duration as "Xh Ym" / "Xm Ys" / "Xs" depending on
// magnitude. Whole-minute cases drop the trailing "0s" so 1m reads as
// "1m" not "1m 0s".
func fmtDuration(d time.Duration) string {
	d = d.Round(time.Second)
	totalSec := int64(d / time.Second)
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}
	if totalSec < 3600 {
		m := totalSec / 60
		s := totalSec % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	return fmt.Sprintf("%dh %dm", h, m)
}

// fmtK renders 1234 as "1.2k", 45678 as "46k", 542 stays "542". The 0
// case is special: instead of "0" we emit "" so a "0 tokens" segment can
// vanish cleanly from the status bar instead of cluttering it.
func fmtK(n int64) string {
	if n == 0 {
		return ""
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 10000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%dk", n/1000)
}

// fmtTokens is an alias kept for consistency with the Hermes Agent
// `fmtTokens` naming used in our analysis docs.
func fmtTokens(n int64) string { return fmtK(n) }
