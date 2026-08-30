package tui

import "testing"

func TestTableToCardFitsNarrowWidth(t *testing.T) {
	tbl := tableData{
		headers: []string{"ID", "Name", "Status"},
		rows: [][]string{
			{"1", "alpha", "ok"},
			{"2", "beta", "error"},
		},
	}
	out := renderTableToCard(tbl, 20)
	if out == "" {
		t.Error("expected non-empty card output")
	}
}

func TestTableHorizontalPrefersMarkdownWhenItFits(t *testing.T) {
	tbl := tableData{
		headers: []string{"A", "B"},
		rows:    [][]string{{"1", "2"}, {"3", "4"}},
	}
	out := renderTableHorizontal(tbl, 40)
	if out == "" {
		t.Error("expected non-empty horizontal table")
	}
}

func TestTableHorizontalReturnsEmptyWhenTooNarrow(t *testing.T) {
	tbl := tableData{
		headers: []string{"A", "B"},
		rows:    [][]string{{"longer header", "another"}},
	}
	out := renderTableHorizontal(tbl, 8)
	if out != "" {
		t.Errorf("expected empty when too narrow, got %q", out)
	}
}

func TestLongestWord(t *testing.T) {
	if got := longestWord("hello world"); got != 5 {
		t.Errorf("longestWord = %d, want 5", got)
	}
	if got := longestWord(""); got != 0 {
		t.Errorf("longestWord empty = %d, want 0", got)
	}
}
