package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runDeadCodeTool(t *testing.T, workingDir string, params DeadCodeParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewDeadCodeTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  DeadCodeToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func writeDeadCodeFile(t *testing.T, dir, name, src string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
}

func TestDeadCodeToolReportsCandidatesRelativeToTheWorkingDir(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "pkg/a.go", `package pkg

func Orphan() {}
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "pkg/a.go:3")
	require.Contains(t, resp.Content, "func Orphan")
	// The path must be relative to the working directory, not absolute.
	require.NotContains(t, resp.Content, dir)
}

// The model has to be told these are candidates, or it will happily delete
// a method that only exists to satisfy an interface.
func TestDeadCodeToolWarnsThatResultsAreCandidates(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func Orphan() {}
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	require.Contains(t, resp.Content, "candidates to review")
	require.Contains(t, resp.Content, "interface")
}

func TestDeadCodeToolSaysSoWhenNothingIsUnreferenced(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func used() {}

func main() { used() }
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	require.Contains(t, resp.Content, "No unreferenced declarations found")
}

func TestDeadCodeToolReportsNoGoFiles(t *testing.T) {
	resp := runDeadCodeTool(t, t.TempDir(), DeadCodeParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestDeadCodeToolResolvesARelativePathAgainstTheWorkingDir(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "keep/a.go", `package keep

func KeepOrphan() {}
`)
	writeDeadCodeFile(t, dir, "skip/b.go", `package skip

func SkipOrphan() {}
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{Path: "keep"})
	require.Contains(t, resp.Content, "KeepOrphan")
	require.NotContains(t, resp.Content, "SkipOrphan")
}

func TestDeadCodeToolDefaultsToIncludingUnexported(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func unexportedOrphan() {}
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	require.Contains(t, resp.Content, "unexportedOrphan")

	no := false
	resp = runDeadCodeTool(t, dir, DeadCodeParams{IncludeUnexported: &no})
	require.NotContains(t, resp.Content, "unexportedOrphan")
}

func TestDeadCodeToolDefaultsToExcludingTests(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func OnlyTestsUseMe() {}
`)
	writeDeadCodeFile(t, dir, "a_test.go", `package a

import "testing"

func TestIt(t *testing.T) { OnlyTestsUseMe() }
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	require.Contains(t, resp.Content, "OnlyTestsUseMe")
	require.Contains(t, resp.Content, "Test files were not scanned")

	yes := true
	resp = runDeadCodeTool(t, dir, DeadCodeParams{IncludeTests: &yes})
	require.NotContains(t, resp.Content, "OnlyTestsUseMe")
}

func TestDeadCodeToolErrorsOnAMissingPath(t *testing.T) {
	resp := runDeadCodeTool(t, t.TempDir(), DeadCodeParams{Path: "does-not-exist"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "cannot scan")
}

func TestDeadCodeToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", `package a

func Orphan() {}
`)

	resp := runDeadCodeTool(t, dir, DeadCodeParams{})
	var meta DeadCodeResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Candidates)
	require.Equal(t, 1, meta.FilesScanned)
	require.False(t, meta.Truncated)
}
