package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OSC 11/10 background probe. Hermes writes the probe to stdout and
// reads the reply off stdin in a pre-loop. Atlas uses the same
// scheme — write `\x1b]11;?\x07` to stdout, then read the next stdin
// line that looks like an OSC reply. The reply is parsed via
// parseOscColor into an RGB triple, which the App then maps onto the
// detected LightMode.

// oscProbeTimeout is the budget for waiting on the terminal's reply.
// Modern terminals answer within ~5ms; we use 200ms to be safe.
const oscProbeTimeout = 200 * time.Millisecond

// parseOscColor parses an X11 color reply. Three accepted formats:
//
//	"rgb:RRRR/GGGG/BBBB" — 1-4 hex digits per channel, scaled
//	"rgba:RRRR/GGGG/BBBB/AAAA" — alpha dropped
//	"#rrggbb" / "rrggbb" — 3- or 6-digit hex (Atlas extension)
//
// The "rgb:" form is what real terminals emit; the "#" form is the
// Atlas-internal canonical form.
func parseOscColor(data string) (r, g, b uint8, ok bool) {
	data = strings.TrimSpace(data)
	if data == "" {
		return 0, 0, 0, false
	}
	if strings.HasPrefix(data, "#") || isHex(data) {
		return parseHexColor(data)
	}
	if strings.HasPrefix(data, "rgb:") {
		return parseX11RGB(strings.TrimPrefix(data, "rgb:"))
	}
	if strings.HasPrefix(data, "rgba:") {
		// Drop alpha.
		parts := strings.Split(strings.TrimPrefix(data, "rgba:"), "/")
		if len(parts) < 3 {
			return 0, 0, 0, false
		}
		return parseX11RGB(strings.Join(parts[:3], "/"))
	}
	return 0, 0, 0, false
}

// parseX11RGB parses the 1-4-digit hex channel format emitted by
// xterm et al. The trick: any number of hex digits per channel, with
// the value scaled to 0-255 by the formula `value * 255 / (16^n - 1)`.
func parseX11RGB(s string) (r, g, b uint8, ok bool) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	vals := []uint8{0, 0, 0}
	for i, p := range parts {
		v, perr := parseX11Channel(p)
		if perr != nil {
			return 0, 0, 0, false
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], true
}

func parseX11Channel(s string) (uint8, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, err
	}
	// Scale to 0-255: value * 255 / (16^n - 1).
	n := uint64(len(s))
	denom := uint64(1)<<(4*n) - 1
	return uint8(uint64(v) * 255 / denom), nil
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}

func parseHexColor(s string) (r, g, b uint8, ok bool) {
	h := strings.TrimPrefix(s, "#")
	switch len(h) {
	case 3:
		// "abc" → "aabbcc"
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		fallthrough
	case 6:
		v, err := strconv.ParseUint(h, 16, 32)
		if err != nil {
			return 0, 0, 0, false
		}
		return uint8(v >> 16), uint8(v >> 8), uint8(v), true
	case 12:
		// 12-digit (rrggbbaaaaaa) — take the high byte per channel.
		v, err := strconv.ParseUint(h, 16, 64)
		if err != nil {
			return 0, 0, 0, false
		}
		return uint8(v >> 40), uint8(v >> 24), uint8(v >> 8), true
	}
	return 0, 0, 0, false
}

// oscProbeRequest is the sequence the App writes to stdout before
// entering the Bubbletea input loop.
const oscBgProbe = "\x1b]11;?\x07"
const oscFgProbe = "\x1b]10;?\x07"

// parseOscReply matches a single line of stdin against the OSC reply
// grammar. Returns the parsed color triple, or false.
func parseOscReply(line string) (uint8, uint8, uint8, bool) {
	// Format: "\x1b]11;rgb:RRRR/GGGG/BBBB\x07" or "\x1b]10;...;ST".
	// We strip the leading ESC ] <code>; and trailing terminator.
	if !strings.HasPrefix(line, "\x1b]") {
		return 0, 0, 0, false
	}
	body := line[2:]
	// Find the semicolon separating code from payload.
	semi := strings.Index(body, ";")
	if semi < 0 {
		return 0, 0, 0, false
	}
	// payload is everything after the semi up to the terminator.
	payload := body[semi+1:]
	// Trim the terminator: BEL (\x07) or ST (ESC \).
	if strings.HasSuffix(payload, "\x07") {
		payload = payload[:len(payload)-1]
	} else if strings.HasSuffix(payload, "\x1b\\") {
		payload = payload[:len(payload)-2]
	}
	return parseOscColor(payload)
}
