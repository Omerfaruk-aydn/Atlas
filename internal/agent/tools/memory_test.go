package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
	"github.com/stretchr/testify/require"
)

func newMemoryTool(t *testing.T, opts memory.Options) fantasy.AgentTool {
	t.Helper()
	dir := t.TempDir()
	if opts.ProjectDir == "" {
		opts.ProjectDir = filepath.Join(dir, "project")
	}
	if opts.UserDir == "" {
		opts.UserDir = filepath.Join(dir, "user")
	}
	return NewMemoryTool(memory.New(opts))
}

func runMemory(t *testing.T, tool fantasy.AgentTool, params MemoryParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	res, err := tool.Run(t.Context(), fantasy.ToolCall{
		ID:    "test-call",
		Name:  MemoryToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return res
}

func TestMemoryToolAddsAndReadsBack(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{
		Action: "add",
		Scope:  "project",
		Entry:  "the race detector needs a C compiler on Windows",
	})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "the race detector needs a C compiler")
	require.Contains(t, res.Content, "next session",
		"the model is told the write is not visible in this conversation")
}

func TestMemoryToolReportsAnUnknownScopeToTheModel(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "add", Scope: "everything", Entry: "x"})

	require.True(t, res.IsError, "a fixable mistake comes back as a result, not as a failed turn")
	require.Contains(t, res.Content, "unknown scope")
}

func TestMemoryToolReportsAnUnknownAction(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "append", Scope: "project", Entry: "x"})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "unknown action")
}

func TestMemoryToolTellsTheModelHowMuchToCut(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{ProjectLimit: 20})

	res := runMemory(t, tool, MemoryParams{
		Action: "add",
		Scope:  "project",
		Entry:  strings.Repeat("x", 100),
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "over the 20 limit by",
		"the model is told the size of the problem, not just that there is one")
}

func TestMemoryToolRefusesAnAmbiguousReplace(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "go 1.26"})
	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "go 1.26 is pinned"})

	res := runMemory(t, tool, MemoryParams{
		Action: "replace", Scope: "project", Old: "go 1.26", New: "go 1.27",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "more than once")
}

func TestMemoryToolSetConsolidates(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "user", Entry: "one"})
	runMemory(t, tool, MemoryParams{Action: "add", Scope: "user", Entry: "two"})

	res := runMemory(t, tool, MemoryParams{
		Action: "set", Scope: "user", Entry: "- one and two, together",
	})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "one and two, together")
	require.NotContains(t, res.Content, "- one\n")
}

func TestMemoryToolShowsAnEmptyStoreAsEmpty(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "only"})
	res := runMemory(t, tool, MemoryParams{Action: "remove", Scope: "project", Old: "only"})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "(empty)")
}

func TestMemoryToolReportsUsage(t *testing.T) {
	t.Parallel()
	tool := newMemoryTool(t, memory.Options{})

	res := runMemory(t, tool, MemoryParams{Action: "add", Scope: "project", Entry: "abc"})

	require.Contains(t, res.Content, "Using 6 of")
}
