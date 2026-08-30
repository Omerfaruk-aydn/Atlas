package tui

import "testing"

// messageGroup maps "user" → GroupUser, "assistant" → GroupModel,
// "info"/"tool"/"error" → GroupNote, plus kind-driven overrides.
func TestMessageGroupRoles(t *testing.T) {
	cases := []struct {
		role, kind string
		want       BlockGroup
	}{
		{"user", "", GroupUser},
		{"assistant", "", GroupModel},
		{"info", "", GroupNote},
		{"tool", "", GroupNote},
		{"error", "", GroupNote},
		{"assistant", "trail", GroupTrail},
		{"user", "event", GroupEvent},
		{"assistant", "intro", GroupIntro},
		{"user", "slash", GroupSlash},
		{"user", "diff", GroupDiff},
	}
	for _, c := range cases {
		if got := messageGroup(c.role, c.kind); got != c.want {
			t.Errorf("messageGroup(%q, %q) = %d, want %d", c.role, c.kind, got, c.want)
		}
	}
}

// hasLeadGap: same group → no gap. Different group, neither self-spaced
// nor trailing-gap-painting → gap. Self-spaced current → never. Trail
// predecessor with trailing gap → no double gap.
func TestHasLeadGapRules(t *testing.T) {
	cases := []struct {
		prev, cur BlockGroup
		want      bool
		desc      string
	}{
		{GroupModel, GroupModel, false, "same group → no gap"},
		{GroupModel, GroupUser, false, "user is self-spaced → no gap"},
		{GroupUser, GroupModel, false, "user paints trailing gap → no double gap"},
		{GroupModel, GroupNote, true, "model → note gets a gap"},
		{GroupNote, GroupModel, true, "note → model gets a gap"},
		{GroupIntro, GroupUser, false, "first user is self-spaced"},
		{GroupUser, GroupEvent, false, "event is self-spaced → no gap"},
		{GroupModel, GroupTrail, true, "model → trail gets a gap"},
		{GroupTrail, GroupModel, true, "trail → model gets a gap (no trailing-paint on trail)"},
	}
	for _, c := range cases {
		if got := hasLeadGap(c.prev, c.cur); got != c.want {
			t.Errorf("%s: hasLeadGap(%d, %d) = %v, want %v", c.desc, c.prev, c.cur, got, c.want)
		}
	}
}

// visibleGroup walks back over trail rows to find the real predecessor.
func TestVisibleGroupSkipsInvisibleTrail(t *testing.T) {
	msgs := []chatMessage{
		{role: "user", text: "hi"},
		{role: "trail", text: ""},     // invisible
		{role: "trail", text: ""},     // invisible
		{role: "assistant", text: "reply"},
	}
	// visibleGroup looks at the predecessor of index i, so we pass the
	// assistant's index (3) and expect the walker to skip past the two
	// invisible trails and surface the user row.
	got, ok := visibleGroup(msgs, 3)
	if !ok {
		t.Fatal("expected to find a visible predecessor")
	}
	if got != GroupUser {
		t.Errorf("expected GroupUser (skipping two invisible trails), got %d", got)
	}
}
