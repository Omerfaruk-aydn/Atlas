package tui

import "testing"

// detectLightMode env-driven cascade — explicit env wins.
func TestDetectLightModeExplicitEnv(t *testing.T) {
	env := []string{"HERMES_TUI_LIGHT=1"}
	if got := detectLightModeFromEnv(env); got != LightLight {
		t.Errorf("HERMES_TUI_LIGHT=1 should resolve to LightLight, got %d", got)
	}
	env = []string{"HERMES_TUI_LIGHT=0"}
	if got := detectLightModeFromEnv(env); got != LightDark {
		t.Errorf("HERMES_TUI_LIGHT=0 should resolve to LightDark, got %d", got)
	}
	env = []string{"HERMES_TUI_THEME=light"}
	if got := detectLightModeFromEnv(env); got != LightLight {
		t.Errorf("HERMES_TUI_THEME=light should resolve to LightLight, got %d", got)
	}
	env = []string{"HERMES_TUI_THEME=dark"}
	if got := detectLightModeFromEnv(env); got != LightDark {
		t.Errorf("HERMES_TUI_THEME=dark should resolve to LightDark, got %d", got)
	}
}

// backgroundHexToLight treats pure black as "unset" (transparent-bg
// profiles report OSC 11 as #000000 — the noise signal).
func TestBackgroundHexToLightPureBlackUnset(t *testing.T) {
	if got := backgroundHexToLight("#000000"); got != LightUnknown {
		t.Errorf("pure black should be LightUnknown, got %d", got)
	}
	if got := backgroundHexToLight("#ffffff"); got != LightLight {
		t.Errorf("white should be LightLight, got %d", got)
	}
	if got := backgroundHexToLight("#101014"); got != LightDark {
		t.Errorf("near-black should be LightDark, got %d", got)
	}
}

// shouldDowngradeAppleTerminalTruecolor: only when TERM_PROGRAM is
// Apple_Terminal AND the user hasn't explicitly forced truecolor.
func TestShouldDowngradeAppleTerminalTruecolor(t *testing.T) {
	env := []string{"TERM_PROGRAM=Apple_Terminal", "COLORTERM=truecolor"}
	if !shouldDowngradeAppleTerminalTruecolor(env) {
		t.Error("Apple Terminal + no override should downgrade")
	}
	env = []string{"TERM_PROGRAM=Apple_Terminal", "HERMES_TUI_TRUECOLOR=1"}
	if shouldDowngradeAppleTerminalTruecolor(env) {
		t.Error("Apple Terminal + HERMES_TUI_TRUECOLOR=1 should not downgrade")
	}
	env = []string{"TERM_PROGRAM=iTerm.app"}
	if shouldDowngradeAppleTerminalTruecolor(env) {
		t.Error("non-Apple terminals should not be downgraded")
	}
}

// shouldForceTruecolor: only the explicit env values.
func TestShouldForceTruecolor(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"1", true},
		{"true", true},
		{"yes", true},
		{"on", true},
		{"false", false},
		{"nope", false},
	}
	for _, c := range cases {
		got := shouldForceTruecolor([]string{"HERMES_TUI_TRUECOLOR=" + c.env})
		if got != c.want {
			t.Errorf("HERMES_TUI_TRUECOLOR=%q → %v, want %v", c.env, got, c.want)
		}
	}
}
