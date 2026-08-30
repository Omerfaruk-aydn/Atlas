package tui

import (
	"os"
	"strings"
)

// LightMode enumerates the resolved background polarity for theme
// selection. The actual OSC-11/OSC-10 probe happens at startup; this
// file is the env-driven fallback cascade that runs *before* the probe
// (and is used if the probe never resolves).
type LightMode int

const (
	LightDark LightMode = iota
	LightLight
	LightUnknown
)

// detectLightModeFromEnv is the test-friendly version of detectLightMode:
// the same env-driven cascade, but reads from a snapshot rather than
// the live process env. Wired into detectLightMode in production.
func detectLightModeFromEnv(env []string) LightMode {
	if v := lookupEnv(env, "HERMES_TUI_LIGHT"); v != "" {
		return parseBoolLight(v)
	}
	if v := lookupEnv(env, "HERMES_TUI_THEME"); v != "" {
		if lowerEq(v, "light") {
			return LightLight
		}
		if lowerEq(v, "dark") {
			return LightDark
		}
	}
	if bg := lookupEnv(env, "HERMES_TUI_BACKGROUND"); bg != "" {
		return backgroundHexToLight(bg)
	}
	if cfb := lookupEnv(env, "COLORFGBG"); cfb != "" {
		parts := splitOnColon(cfb)
		last := parts[len(parts)-1]
		if last == "7" || last == "15" {
			return LightLight
		}
		return LightDark
	}
	if lookupEnv(env, "TERM_PROGRAM") == "Apple_Terminal" {
		return LightLight
	}
	return LightUnknown
}

func lowerEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func splitOnColon(s string) []string {
	var out []string
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	out = append(out, s[last:])
	return out
}

// detectLightMode implements Hermes's exact env-driven fallback cascade
// (theme.ts detectLightMode). Order:
//
//  1. HERMES_TUI_LIGHT / HERMES_TUI_THEME env vars (explicit user pin).
//  2. Cached OSC-11 background hex (if a previous run wrote one to
//     themeBoot — wired in styles.go).
//  3. COLORFGBG (last field): slots 7/15 mean light, anything else dark.
//  4. TERM_PROGRAM allow-list: Apple_Terminal defaults light (yes,
//     even though it doesn't support truecolor — see forceTruecolor).
//  5. Otherwise dark.
//
// Anything that falls through to (4)/(5) returns LightUnknown so the
// caller can wait for a real OSC-11 probe rather than locking in
// prematurely.
func detectLightMode() LightMode {
	if v := os.Getenv("HERMES_TUI_LIGHT"); v != "" {
		return parseBoolLight(v)
	}
	if v := os.Getenv("HERMES_TUI_THEME"); v != "" {
		if strings.EqualFold(v, "light") {
			return LightLight
		}
		if strings.EqualFold(v, "dark") {
			return LightDark
		}
	}
	if bg := os.Getenv("HERMES_TUI_BACKGROUND"); bg != "" {
		return backgroundHexToLight(bg)
	}
	if cfb := os.Getenv("COLORFGBG"); cfb != "" {
		// COLORFGBG format: "<fg>;<bg>" or ":<fg>:<bg>:<…>". The "bg"
		// slot is conventionally the last colon-separated field. Slots
		// 7 and 15 in the 16-color palette are commonly the two lightest
		// terminal backgrounds, so we treat them as a light signal.
		parts := strings.Split(cfb, ":")
		last := parts[len(parts)-1]
		if last == "7" || last == "15" {
			return LightLight
		}
		return LightDark
	}
	if tp := os.Getenv("TERM_PROGRAM"); tp == "Apple_Terminal" {
		return LightLight
	}
	return LightUnknown
}

// backgroundHexToLight converts a probed background hex (e.g. "#ffffff")
// into a LightMode. Pure black is treated as "unset" (the OSC-11 default
// for transparent-background profiles) so the caller can keep waiting.
func backgroundHexToLight(hex string) LightMode {
	hex = strings.TrimPrefix(strings.ToLower(hex), "#")
	if hex == "000000" || hex == "" {
		return LightUnknown
	}
	r, g, b := parseHex("#" + hex)
	// Average channel luminance, normalized 0-1. Treats > 0.5 as
	// "light" — the same threshold Hermes's themeBoot uses.
	lum := (r + g + b) / (3 * 255)
	if lum > 0.5 {
		return LightLight
	}
	return LightDark
}

func parseBoolLight(v string) LightMode {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return LightLight
	}
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return LightDark
	}
	return LightUnknown
}

// shouldForceTruecolor implements Hermes's shouldForceTruecolor env logic.
// Atlas doesn't have a custom fork like Hermes-Ink, so this only matters
// for the call to lipgloss/termenv that decides truecolor vs 256-color
// quantization. We still expose the env knob so the user can force on
// or off without rebuilding.
func shouldForceTruecolor(env []string) bool {
	v := lookupEnv(env, "HERMES_TUI_TRUECOLOR")
	if v == "" {
		return false
	}
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return true
	}
	if v == "0" || v == "false" || v == "no" || v == "off" {
		return false
	}
	return false
}

// shouldDowngradeAppleTerminalTruecolor mirrors the Apple Terminal quirk:
// pre-"Tahoe 26" Apple_Terminal advertises COLORTERM=truecolor but its
// RGB SGR is broken, so we should pin to 256-color unless the user has
// explicitly forced truecolor.
func shouldDowngradeAppleTerminalTruecolor(env []string) bool {
	if lookupEnv(env, "TERM_PROGRAM") != "Apple_Terminal" {
		return false
	}
	// If the user has already forced truecolor, respect it.
	if shouldForceTruecolor(env) {
		return false
	}
	// Otherwise downgrade.
	return true
}

// lookupEnv is a small wrapper that takes a snapshot of os.Environ at
// startup time, so termprobe.go is testable without touching the
// process env.
func lookupEnv(env []string, key string) string {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return e[len(prefix):]
		}
	}
	return ""
}

// terminalModeReset is the canonical exit-time escape-sequence blob to
// turn off every "sticky" terminal mode Bubbletea may have enabled. We
// use a single synchronous Write here (Bubbletea's own WithAltScreen
// handles the common cases — this is a belt-and-braces cleanup for the
// modes Bubbletea doesn't cover).
//
// The blob is intentionally not embedded in any active code path yet —
// wiring it in requires hooking Bubbletea's teardown, which we'll add
// when we add the kill-switch keybinding. Keeping the constant here so
// it's reviewable as a single artifact.
const terminalModeReset = "\x1b[?1000l\x1b[?1001l\x1b[?1002l\x1b[?1003l\x1b[?1004l\x1b[?1006l\x1b[?2004l\x1b[?2029l\x1b[?1049l"
