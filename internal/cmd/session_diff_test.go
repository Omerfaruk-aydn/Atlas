package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiffToolUsageEmpty(t *testing.T) {
	require.Empty(t, diffToolUsage(nil, nil))
}

func TestDiffToolUsageCommonSameCount(t *testing.T) {
	got := diffToolUsage(map[string]int{"view": 3}, map[string]int{"view": 3})
	require.Equal(t, []toolDiffLine{{Name: "view", A: 3, B: 3}}, got)
}

func TestDiffToolUsageOnlyInFirst(t *testing.T) {
	got := diffToolUsage(map[string]int{"bash": 2}, nil)
	require.Equal(t, []toolDiffLine{{Name: "bash", A: 2, B: 0}}, got)
}

func TestDiffToolUsageOnlyInSecond(t *testing.T) {
	got := diffToolUsage(nil, map[string]int{"bash": 2})
	require.Equal(t, []toolDiffLine{{Name: "bash", A: 0, B: 2}}, got)
}

func TestDiffToolUsageChangedCount(t *testing.T) {
	got := diffToolUsage(map[string]int{"view": 1}, map[string]int{"view": 5})
	require.Equal(t, []toolDiffLine{{Name: "view", A: 1, B: 5}}, got)
}

func TestDiffToolUsageSortedByName(t *testing.T) {
	got := diffToolUsage(map[string]int{"view": 1, "bash": 1}, nil)
	require.Equal(t, "bash", got[0].Name)
	require.Equal(t, "view", got[1].Name)
}

func TestPrintToolUsageDiffWithNothing(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printToolUsageDiff(&out, nil, nil))
	require.Contains(t, out.String(), "Neither session called any tools.")
}

func TestPrintToolUsageDiffFormatsEachCase(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printToolUsageDiff(&out,
		map[string]int{"bash": 2, "view": 1, "grep": 3},
		map[string]int{"view": 1, "grep": 5, "edit": 1},
	))
	got := out.String()
	require.Contains(t, got, "- bash: 2 (only in first)\n")
	require.Contains(t, got, "+ edit: 1 (only in second)\n")
	require.Contains(t, got, "  view: 1\n")
	require.Contains(t, got, "~ grep: 3 -> 5\n")
}
