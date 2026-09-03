package config

import (
	"testing"
	"time"
)

func TestToolDebuggerDefaultsToDisabled(t *testing.T) {
	var d ToolDebugger
	if d.IsEnabled() {
		t.Fatal("IsEnabled() = true, want false when unset")
	}
}

func TestToolDebuggerEnabledHonorsExplicitTrue(t *testing.T) {
	enabled := true
	d := ToolDebugger{Enabled: &enabled}
	if !d.IsEnabled() {
		t.Fatal("IsEnabled() = false, want true")
	}
}

func TestToolDebuggerActionTimeoutDefaultsTo30Seconds(t *testing.T) {
	var d ToolDebugger
	if got := d.GetActionTimeout(); got != 30*time.Second {
		t.Fatalf("GetActionTimeout() = %v, want 30s", got)
	}
}

func TestToolDebuggerActionTimeoutHonorsConfiguredValue(t *testing.T) {
	dur := 90 * time.Second
	d := ToolDebugger{ActionTimeout: &dur}
	if got := d.GetActionTimeout(); got != dur {
		t.Fatalf("GetActionTimeout() = %v, want %v", got, dur)
	}
}
