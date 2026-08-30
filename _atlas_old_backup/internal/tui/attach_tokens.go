package tui

import (
	"fmt"
	"regexp"
	"strings"
)

// AttachTokenKind tags one of the two collapsed-token flavors the
// composer can carry: image ([ [ Image N ] ]) and paste snippet
// ([ [ first.. [N lines] .. last ] ]). Atlas's port is a parallel
// data structure to the highlighter's — the highlighter renders the
// colored span, attachTokens owns the expand-at-submit logic.
type AttachTokenKind int

const (
	AttachImage AttachTokenKind = iota
	AttachPaste
)

// AttachToken is one resolved token in the composer buffer.
type AttachToken struct {
	Kind    AttachTokenKind
	Index   int    // 1-based image index, or paste ordinal
	Display string // the literal "[[ ... ]]" string in the buffer
	Payload string // the actual content (for paste) or "" (for image)
	Lines   int    // for paste: line count of the payload
}

// attachImageRe matches "[[ Image N ]]" tokens in the composer buffer.
var attachImageRe = regexp.MustCompile(`\[\[ Image (\d+) \]\]`)

// attachPasteRe matches "[[ first.. [N lines] .. last ]]" paste tokens.
var attachPasteRe = regexp.MustCompile(`\[\[ ([^.]+)\.\. \[(\d+) lines\] \.\. ([^.]+) \]\]`)

// nextImageIndex finds the smallest N>=1 not already present in buffer.
// Mirrors Hermes's nextImageIndex helper.
func nextImageIndex(buffer string) int {
	max := 0
	for _, m := range attachImageRe.FindAllStringSubmatch(buffer, -1) {
		if len(m) >= 2 {
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			if n > max {
				max = n
			}
		}
	}
	return max + 1
}

// droppedTokens returns the tokens that are in the old buffer but no
// longer in the new one. The user "deletes" an attached token by
// removing the bracketed text from the composer; the App then
// forgets the payload. The set of dropped tokens is the diff.
func droppedTokens(oldBuf, newBuf string) []string {
	old := tokenSet(oldBuf)
	var out []string
	for _, tok := range old {
		if !strings.Contains(newBuf, tok) {
			out = append(out, tok)
		}
	}
	return out
}

// tokenSet returns the set of all "[[ ... ]]" tokens in buffer.
func tokenSet(buffer string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range append(append([]string{}, attachImageRe.FindAllString(buffer, -1)...), attachPasteRe.FindAllString(buffer, -1)...) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// expandTokens walks the buffer left-to-right, swapping each
// "[[ Image N ]]" / "[[ first.. [N lines] .. last ]]" token for the
// actual payload. The expansion is order-sensitive (a paste token
// expanded first stays first), and image tokens expand to empty
// strings (the image rides out-of-band in session.attached_images).
//
// Returns (expanded, imageIndices) — imageIndices is the list of N
// values for every image token encountered, in order, for the caller
// to wire into the request payload.
func expandTokens(buffer string, payloads map[string]string) (string, []int) {
	var b strings.Builder
	var images []int
	// Walk the buffer left-to-right, finding the earliest token at
	// each step.
	for {
		idx := findEarliestToken(buffer)
		if idx < 0 {
			b.WriteString(buffer)
			break
		}
		end := tokenEndAt(buffer, idx)
		token := buffer[idx:end]
		if attachImageRe.MatchString(token) {
			m := attachImageRe.FindStringSubmatch(token)
			var n int
			fmt.Sscanf(m[1], "%d", &n)
			images = append(images, n)
			// Image tokens expand to empty — drop any
			// adjacent space left over (clean up the
			// double-space artifact).
			b.WriteString(buffer[:idx])
			// Skip the next char if it's a space (the
			// "consume one adjacent space" rule).
			skip := 0
			if end < len(buffer) && buffer[end] == ' ' {
				skip = 1
			}
			buffer = buffer[end+skip:]
		} else {
			payload := payloads[token]
			if payload == "" {
				// Paste linkage dropped — treat as
				// literal text.
				b.WriteString(buffer[:end])
				buffer = buffer[end:]
			} else {
				b.WriteString(buffer[:idx])
				b.WriteString(payload)
				// Also consume one adjacent space.
				skip := 0
				if end < len(buffer) && buffer[end] == ' ' {
					skip = 1
				}
				buffer = buffer[end+skip:]
			}
		}
	}
	return b.String(), images
}

// findEarliestToken returns the byte index of the earliest image or
// paste token in buffer, or -1 if none. Image tokens are checked
// first so a token like "[[ Image 1 ]]" doesn't get mis-parsed as a
// paste (the image regex is more specific).
func findEarliestToken(buffer string) int {
	imgIdx := indexFirst(attachImageRe, buffer)
	pasteIdx := indexFirst(attachPasteRe, buffer)
	switch {
	case imgIdx < 0:
		return pasteIdx
	case pasteIdx < 0:
		return imgIdx
	default:
		if imgIdx < pasteIdx {
			return imgIdx
		}
		return pasteIdx
	}
}

func indexFirst(re *regexp.Regexp, s string) int {
	loc := re.FindStringIndex(s)
	if loc == nil {
		return -1
	}
	return loc[0]
}

// tokenEndAt returns the byte index just past the token starting at
// start. Assumes start is the result of findEarliestToken on the
// same buffer.
func tokenEndAt(buffer string, start int) int {
	if loc := attachImageRe.FindStringIndex(buffer[start:]); loc != nil && loc[0] == 0 {
		return start + loc[1]
	}
	if loc := attachPasteRe.FindStringIndex(buffer[start:]); loc != nil && loc[0] == 0 {
		return start + loc[1]
	}
	return start
}
