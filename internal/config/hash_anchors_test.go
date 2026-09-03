package config

import "testing"

func TestToolViewHashAnchorsDefaultsOff(t *testing.T) {
	var view ToolView
	if view.HashAnchorsEnabled() {
		t.Fatal("hash anchors must default to off: unset users get the existing view/edit token cost and format")
	}
}

func TestToolViewHashAnchorsEnabled(t *testing.T) {
	enabled := true
	view := ToolView{HashAnchors: &enabled}
	if !view.HashAnchorsEnabled() {
		t.Fatal("hash_anchors: true must enable hash anchors")
	}
}

func TestToolViewHashAnchorsExplicitlyDisabled(t *testing.T) {
	disabled := false
	view := ToolView{HashAnchors: &disabled}
	if view.HashAnchorsEnabled() {
		t.Fatal("hash_anchors: false must stay disabled")
	}
}
