package config

import "testing"

func TestToolTeamsDefaultsToDisabled(t *testing.T) {
	var tm ToolTeams
	if tm.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false when unset")
	}
}

func TestToolTeamsEnabledHonorsExplicitTrue(t *testing.T) {
	enabled := true
	tm := ToolTeams{Enabled: &enabled}
	if !tm.IsEnabled() {
		t.Fatal("IsEnabled() = false, want true")
	}
}
