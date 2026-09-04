package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runAntiPatternScanTool(t *testing.T, workingDir string, params AntiPatternScanParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewAntiPatternScanTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  AntiPatternScanToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const antiPatternFixture = `package a

func f() error {
	err := g()
	if err != nil {
	}
	return nil
}

func g() error { return nil }
`

func TestAntiPatternScanToolReportsFindings(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", antiPatternFixture)

	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 finding(s)")
	require.Contains(t, resp.Content, "swallowed-error")
	require.Contains(t, resp.Content, "not type-checked")
}

func TestAntiPatternScanToolReportsCleanCode(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")

	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No anti-patterns found")
}

func TestAntiPatternScanToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No Go files found")
}

func TestAntiPatternScanToolSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", antiPatternFixture)

	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestAntiPatternScanToolIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", antiPatternFixture)

	include := true
	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{IncludeTests: &include})
	require.Contains(t, resp.Content, "1 finding(s)")
}

func TestAntiPatternScanToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", antiPatternFixture)

	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{})
	var meta AntiPatternScanResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.ByKind["swallowed-error"])
	require.Equal(t, 1, meta.FilesScanned)
}

func TestAntiPatternScanToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", antiPatternFixture)

	resp := runAntiPatternScanTool(t, dir, AntiPatternScanParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 finding(s)")
}
