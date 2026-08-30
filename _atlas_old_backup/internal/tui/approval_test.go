package tui

import "testing"

// NewApprovalOptions returns 3 default options (once/session/always/deny → 4)
func TestNewApprovalOptionsDefault(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	if len(opts) != 4 {
		t.Errorf("expected 4 default options, got %d", len(opts))
	}
	if opts[0].Kind != "once" {
		t.Errorf("first option should be once, got %q", opts[0].Kind)
	}
	if opts[len(opts)-1].Kind != "deny" {
		t.Errorf("last option should be deny, got %q", opts[len(opts)-1].Kind)
	}
}

// tirith-warning mode drops "always".
func TestNewApprovalOptionsTirithWarningDropsAlways(t *testing.T) {
	opts := NewApprovalOptions(false, true)
	for _, o := range opts {
		if o.Kind == "always" {
			t.Error("tirith warning must drop the always option")
		}
	}
}

// smart-denied mode narrows to once/deny only.
func TestNewApprovalOptionsSmartDeniedNarrowsToTwo(t *testing.T) {
	opts := NewApprovalOptions(true, false)
	if len(opts) != 2 {
		t.Errorf("smartDenied should narrow to 2 options, got %d", len(opts))
	}
}

// approvalKeyAction: Enter chooses current selection.
func TestApprovalKeyActionEnterChooses(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction("enter", 1, opts)
	if act.kind != "choose" || act.idx != 1 {
		t.Errorf("enter at 1 → choose(1), got %+v", act)
	}
}

// approvalKeyAction: number quick-pick maps to option index.
func TestApprovalKeyActionNumberQuickPick(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction("2", 0, opts)
	if act.kind != "choose" || act.idx != 1 {
		t.Errorf("'2' at sel 0 → choose(1), got %+v", act)
	}
}

// approvalKeyAction: up wraps to the last option.
func TestApprovalKeyActionUpWraps(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction("up", 0, opts)
	if act.kind != "move" || act.idx != len(opts)-1 {
		t.Errorf("up at 0 → move to last (%d), got %+v", len(opts)-1, act)
	}
}

// approvalKeyAction: down wraps to the first option.
func TestApprovalKeyActionDownWraps(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction("down", len(opts)-1, opts)
	if act.kind != "move" || act.idx != 0 {
		t.Errorf("down at last → move to 0, got %+v", act)
	}
}

// approvalKeyAction: n / esc / ctrl+c cancel.
func TestApprovalKeyActionCancel(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	for _, k := range []string{"n", "esc", "ctrl+c"} {
		act := approvalKeyAction(k, 0, opts)
		if act.kind != "cancel" {
			t.Errorf("%q should cancel, got %+v", k, act)
		}
	}
}

// y maps to the first non-deny option.
func TestApprovalKeyActionYSkipsDeny(t *testing.T) {
	opts := NewApprovalOptions(false, false)
	act := approvalKeyAction("y", 0, opts)
	if act.kind != "choose" {
		t.Fatalf("y should choose, got %+v", act)
	}
	if opts[act.idx].Kind == "deny" {
		t.Errorf("y must not pick deny, got %d (%s)", act.idx, opts[act.idx].Kind)
	}
}
