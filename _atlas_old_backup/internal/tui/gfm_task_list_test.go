package tui

import "testing"

func TestRenderGFMTaskListReplacesCheckbox(t *testing.T) {
	in := "- [ ] todo one\n- [x] todo two\n- [X] todo three"
	out := renderGFMTaskList(in)
	if !containsStr(out, "☐ todo one") {
		t.Errorf("expected ☐ for open, got %q", out)
	}
	if !containsStr(out, "☑ todo two") {
		t.Errorf("expected ☑ for done, got %q", out)
	}
	if !containsStr(out, "☑ todo three") {
		t.Errorf("expected ☑ for X (case-insensitive), got %q", out)
	}
}

func TestRenderGFMTaskListLeavesPlainLinesAlone(t *testing.T) {
	in := "regular text\n- not a task\n  - [ ] indented task"
	out := renderGFMTaskList(in)
	if !containsStr(out, "- not a task") {
		t.Errorf("expected plain dash line to pass through, got %q", out)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
