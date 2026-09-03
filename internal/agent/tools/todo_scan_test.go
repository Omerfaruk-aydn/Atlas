package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runTodoScanTool(t *testing.T, workingDir string, params TodoScanParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewTodoScanTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  TodoScanToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const todoFixture = `package a

// TODO(alice): finish this, see #1234
// FIXME: known to break on empty input
// HACK: works around a bug upstream
`

func TestTodoScanToolListsMarkersWithTheirDetail(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", todoFixture)

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "3 marker(s)")
	require.Contains(t, resp.Content, "TODO(alice)")
	require.Contains(t, resp.Content, "[#1234]")
	require.Contains(t, resp.Content, "known to break on empty input")
}

// A raw total says nothing; the mix of kinds is what distinguishes
// aspirational notes from recorded defects.
func TestTodoScanToolLeadsWithTheBreakdown(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", todoFixture)

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.Contains(t, resp.Content, "by kind:")
	require.Contains(t, resp.Content, "1 have an owner, 1 reference a ticket")
}

func TestTodoScanToolFiltersByKind(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", todoFixture)

	resp := runTodoScanTool(t, dir, TodoScanParams{Kinds: []string{"FIXME"}})
	require.Contains(t, resp.Content, "1 marker(s)")
	require.Contains(t, resp.Content, "FIXME")
	require.NotContains(t, resp.Content, "HACK")
}

// The pattern must not fire on identifiers or prose, or the tool is
// useless on any project that manages a todo list.
func TestTodoScanToolIgnoresIdentifiersAndProse(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

// This renders the user's todos.
func renderTodos(todoList []string) {}
`)

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.Contains(t, resp.Content, "No markers found")
}

func TestTodoScanToolReportsPathsRelativeToTheWorkingDir(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "pkg/a.go", "package a\n\n// TODO: here\n")

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.Contains(t, resp.Content, "pkg/a.go")
	require.NotContains(t, resp.Content, dir)
}

// A count read as a task list is the standard misuse of this output.
func TestTodoScanToolSaysACountIsNotATaskList(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", todoFixture)

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.Contains(t, resp.Content, "not a task list")
	require.Contains(t, resp.Content, "aspirational")
}

func TestTodoScanToolRestrictsByExtension(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\n// TODO: go one\n")
	writeDeadCodeFile(t, dir, "a.py", "# TODO: python one\n")

	resp := runTodoScanTool(t, dir, TodoScanParams{Extensions: []string{"go"}})
	require.Contains(t, resp.Content, "go one")
	require.NotContains(t, resp.Content, "python one")
}

func TestTodoScanToolHonoursIncludeTests(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\n// TODO: production\n")
	writeDeadCodeFile(t, dir, "a_test.go", "package a\n\n// TODO: in a test\n")

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.NotContains(t, resp.Content, "in a test")

	yes := true
	resp = runTodoScanTool(t, dir, TodoScanParams{IncludeTests: &yes})
	require.Contains(t, resp.Content, "in a test")
}

func TestTodoScanToolReportsNothingScannable(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "data.bin", "TODO: not a source file\n")

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	require.Contains(t, resp.Content, "No scannable files")
	require.Contains(t, resp.Content, "extensions")
}

func TestTodoScanToolErrorsOnAMissingPath(t *testing.T) {
	resp := runTodoScanTool(t, t.TempDir(), TodoScanParams{Path: "nope"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestTodoScanToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", todoFixture)

	resp := runTodoScanTool(t, dir, TodoScanParams{})
	var meta TodoScanResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 3, meta.Total)
	require.Equal(t, 1, meta.Owned)
	require.Equal(t, 1, meta.Ticketed)
	require.Equal(t, 1, meta.ByKind["TODO"])
	require.Equal(t, 1, meta.ByKind["FIXME"])
	require.Equal(t, 1, meta.FilesScanned)
}

func TestTruncateTextLeavesShortTextAlone(t *testing.T) {
	require.Equal(t, "short", truncateText("short", 10))
	require.Equal(t, "abc...", truncateText("abcdef", 3))
}
