package cmd

import (
	"bytes"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
	"github.com/stretchr/testify/require"
)

const memoryFixture = "Deploy steps\nrun make release\nthen tag it\n"

func TestSearchMemoryIsCaseInsensitive(t *testing.T) {
	got := searchMemory(memory.ScopeProject, memoryFixture, "DEPLOY")
	require.Len(t, got, 1)
	require.Equal(t, 1, got[0].Line)
	require.Equal(t, "Deploy steps", got[0].Text)
	require.Equal(t, memory.ScopeProject, got[0].Scope)
}

func TestSearchMemoryReportsEveryMatchingLine(t *testing.T) {
	got := searchMemory(memory.ScopeUser, "tag one\nnothing\ntag two\n", "tag")
	require.Len(t, got, 2)
	require.Equal(t, 1, got[0].Line)
	require.Equal(t, 3, got[1].Line)
}

// An empty query would otherwise match every line, which is `memory show`.
func TestSearchMemoryIgnoresABlankQuery(t *testing.T) {
	require.Empty(t, searchMemory(memory.ScopeProject, memoryFixture, ""))
	require.Empty(t, searchMemory(memory.ScopeProject, memoryFixture, "   "))
}

func TestPrintMemoryMatchesWithNone(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printMemoryMatches(&out, nil, "kubernetes"))
	require.Contains(t, out.String(), `No memory lines match "kubernetes".`)
}

func TestPrintMemoryMatchesShowsScopeAndLine(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, printMemoryMatches(&out, []memoryMatch{
		{Scope: memory.ScopeProject, Line: 7, Text: "run make release"},
	}, "release"))
	require.Contains(t, out.String(), "project:7: run make release")
}

func TestMemorySearchEndToEnd(t *testing.T) {
	workingDir := t.TempDir()
	dataDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{}`)

	c := newSkillTestCmd(t, runMemorySearch, workingDir, dataDir)
	var out bytes.Buffer
	c.SetOut(&out)

	// Nothing has been written to either store yet, so this proves the
	// command reads them without failing on absent files.
	require.NoError(t, c.RunE(c, []string{"anything"}))
	require.Contains(t, out.String(), "No memory lines match")
}
