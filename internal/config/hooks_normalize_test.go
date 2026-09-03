package config

import "testing"

func TestNormalizeHookEvent(t *testing.T) {
	cases := map[string]string{
		"PreToolUse":         "PreToolUse",
		"pretooluse":         "PreToolUse",
		"pre_tool_use":       "PreToolUse",
		"PostToolUse":        "PostToolUse",
		"post_tool_use":      "PostToolUse",
		"UserPromptSubmit":   "UserPromptSubmit",
		"user_prompt_submit": "UserPromptSubmit",
		"SessionStart":       "SessionStart",
		"session_start":      "SessionStart",
		"SessionEnd":         "SessionEnd",
		"session_end":        "SessionEnd",
		"PreCompact":         "PreCompact",
		"pre_compact":        "PreCompact",
		"something-unknown":  "something-unknown",
	}
	for in, want := range cases {
		if got := normalizeHookEvent(in); got != want {
			t.Errorf("normalizeHookEvent(%q) = %q, want %q", in, got, want)
		}
	}
}
