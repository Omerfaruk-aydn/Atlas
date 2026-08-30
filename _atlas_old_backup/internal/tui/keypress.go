package tui

import (
	"strings"
)

// keypressLayer is the terminal input parser. Bubbletea already
// handles basic key normalization, but a few Hermes-specific things
// still need explicit handling:
//
//   - Bracketed paste (CSI 200~ ... CSI 201~): Hermes's token logic
//     recognizes the start/end markers at the parser layer, buffers
//     everything between as literal text, and emits a single
//     "pasted text" event so the composer doesn't misinterpret
//     embedded control bytes (e.g. an arrow-key-looking escape
//     sequence) as a navigation keypress.
//
//   - Kitty keyboard protocol (CSI u): decodes the bitmask so we can
//     distinguish super (Cmd/Win) from meta (Alt/Option) — a Cmd
//     binding for "delete word" must NOT match Alt+Backspace.
//
//   - Fused control+text bytes: some IMEs (Vietnamese Telex) emit a
//     fused chunk like "\x7fô" (backspace followed by the
//     recomposed character) in a single read. The parser splits it
//     so the printable half isn't lost.
//
// Bubbletea's x/term library already does most of this; the Atlas
// port adds the bracketed-paste watchdog flush (in case the END
// marker is dropped) and a keymap-aware Cmd/Alt disambiguation.

// PasteState tracks whether we're currently inside a bracketed paste
// block. Started=true means we saw CSI 200~ and are buffering.
type PasteState struct {
	Started bool
	Buffer  strings.Builder
	Watchdog int // frames since last byte; reset on End marker
}

// FeedPush appends a chunk of input bytes to the parser. Returns the
// list of events the parser chose to emit.
//
// Events are one of:
//
//   "paste-start"  — saw CSI 200~
//   "paste-text"   — a paste body chunk (one event per flush at CSI 201~)
//   "paste-end"    — saw CSI 201~ (or the watchdog flushed)
//   "key"          — a non-paste keypress
//   "fused-split"  — a control byte fused with printable text was split
func (p *PasteState) FeedPush(chunk string) []string {
	if p.Started {
		// Inside a paste block: everything is literal text, except
		// for the END marker (CSI 201~) and the watchdog flush.
		if end := strings.Index(chunk, "\x1b[201~"); end >= 0 {
			p.Buffer.WriteString(chunk[:end])
			events := []string{"paste-end", p.Buffer.String()}
			p.Buffer.Reset()
			p.Started = false
			p.Watchdog = 0
			return events
		}
		p.Buffer.WriteString(chunk)
		p.Watchdog++
		if p.Watchdog > 32 {
			// Watchdog: terminal dropped the end marker. Flush
			// what we have and resume normal parsing.
			events := []string{"paste-end", p.Buffer.String()}
			p.Buffer.Reset()
			p.Started = false
			p.Watchdog = 0
			return events
		}
		return nil
	}
	// Outside a paste: look for the start marker.
	if start := strings.Index(chunk, "\x1b[200~"); start >= 0 {
		// Emit the pre-start chunk as a regular key/text stream.
		pre := chunk[:start]
		p.Buffer.Reset()
		p.Buffer.WriteString(chunk[start+6:])
		p.Started = true
		p.Watchdog = 0
		return append(splitFusedControlBytes(pre), "paste-start")
	}
	// No paste markers — just split any fused control+text bytes.
	return splitFusedControlBytes(chunk)
}

// splitFusedControlBytes walks chunk and emits one event per logical
// key, splitting bytes like "\x7fô" (backspace + 'ô') into a separate
// backspace and printable. CR/LF are preserved as their own events
// (Enter semantics matter). Other control bytes (except \t) are
// emitted as their own key events too.
func splitFusedControlBytes(chunk string) []string {
	if chunk == "" {
		return nil
	}
	var out []string
	var buf strings.Builder
	flushBuf := func() {
		if buf.Len() > 0 {
			out = append(out, buf.String())
			buf.Reset()
		}
	}
	for i := 0; i < len(chunk); i++ {
		c := chunk[i]
		switch c {
		case 0x7f:
			// Backspace.
			flushBuf()
			out = append(out, "backspace")
		case '\r', '\n':
			flushBuf()
			out = append(out, string(c))
		case '\t':
			buf.WriteByte(c)
		default:
			if c < 0x20 {
				flushBuf()
				out = append(out, string(c))
			} else {
				buf.WriteByte(c)
			}
		}
	}
	flushBuf()
	return out
}

// decodeKittyModifier extracts the modifier bitmask from a Kitty
// keyboard protocol sequence. The base-10 `mod` field encodes
// shift=1, meta/alt=2, ctrl=4, super/cmd=8 — matching xterm's
// modifyOtherKeys encoding. Returns -1 when the input isn't a
// CSI-u sequence.
//
// Atlas's port uses this to disambiguate Cmd from Alt on macOS
// before binding a key. The App passes the result to the keymap
// matcher so "Cmd+Backspace = kill-line" doesn't accidentally fire
// for Alt+Backspace.
func decodeKittyModifier(seq string) int {
	// CSI <code> ; <mod> u
	if !strings.HasPrefix(seq, "\x1b[") {
		return -1
	}
	rest := strings.TrimPrefix(seq, "\x1b[")
	if !strings.HasSuffix(rest, "u") {
		return -1
	}
	rest = strings.TrimSuffix(rest, "u")
	parts := strings.Split(rest, ";")
	if len(parts) != 2 {
		return -1
	}
	mod, ok := parseInt(parts[1])
	if !ok {
		return -1
	}
	return mod
}

// parseInt is a tiny strconv-free int parser; the keymap layer
// doesn't want to import strconv just for one call.
func parseInt(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// isPasteEvent is a tiny helper the App uses to route a
// "paste-end" event into the composer (vs. the keymap matcher).
func isPasteEvent(event string) bool {
	return event == "paste-start" || event == "paste-end" || event == "paste-text"
}
