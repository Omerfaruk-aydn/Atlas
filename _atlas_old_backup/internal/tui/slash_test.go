package tui

import "testing"

// parseSlashInput splits "/model gpt-4" into ("model", "gpt-4").
func TestParseSlashInput(t *testing.T) {
	cases := []struct {
		in        string
		name, arg string
	}{
		{"", "", ""},
		{"/", "", ""},
		{"/model", "model", ""},
		{"/model gpt-4", "model", "gpt-4"},
		{"model", "model", ""},
		{"  /model  ", "model", ""},
		{"/model gpt-4 with spaces", "model", "gpt-4 with spaces"},
	}
	for _, c := range cases {
		n, a := parseSlashInput(c.in)
		if n != c.name || a != c.arg {
			t.Errorf("parseSlashInput(%q) = (%q, %q), want (%q, %q)", c.in, n, a, c.name, c.arg)
		}
	}
}

// completionToApplyOnSubmit: trailing-space-only addition is a no-op.
func TestCompletionToApplyOnSubmit(t *testing.T) {
	// Already complete + the popover would only append a space: no-op.
	_, isNoOp := completionToApplyOnSubmit("/exit", "/exit ")
	if !isNoOp {
		t.Error("trailing-space-only completion should be a no-op")
	}
	// Genuine replacement.
	apply, isNoOp := completionToApplyOnSubmit("/mo", "/model")
	if isNoOp || apply != "/model" {
		t.Errorf("expected /model replacement, got %q (no-op=%v)", apply, isNoOp)
	}
	// Empty completion is no-op.
	_, isNoOp = completionToApplyOnSubmit("/mo", "")
	if !isNoOp {
		t.Error("empty completion should be no-op")
	}
}

// SlashRegistry.Find resolves names and aliases case-insensitively.
func TestSlashRegistryFind(t *testing.T) {
	reg := newSlashRegistry([]SlashCommand{
		{Name: "model", Aliases: []string{"m"}, Group: "X"},
		{Name: "provider", Aliases: []string{"p"}, Group: "X"},
	})
	if reg.Find("model") == nil {
		t.Error("expected to find /model")
	}
	if reg.Find("M") == nil {
		t.Error("case-insensitive lookup must work")
	}
	if reg.Find("m") == nil {
		t.Error("alias must resolve to the canonical command")
	}
	if reg.Find("nope") != nil {
		t.Error("unknown name must return nil")
	}
}

// SlashRegistry.Grouped groups by Group field, sorted.
func TestSlashRegistryGrouped(t *testing.T) {
	reg := newSlashRegistry([]SlashCommand{
		{Name: "tokens", Group: "Oturum"},
		{Name: "clear", Group: "Çekirdek"},
		{Name: "help", Group: "Çekirdek"},
		{Name: "mem", Group: "Hata Ayıklama"},
	})
	groups, byGroup := reg.Grouped()
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}
	if len(byGroup["Çekirdek"]) != 2 {
		t.Errorf("expected 2 Çekirdek commands, got %d", len(byGroup["Çekirdek"]))
	}
}
