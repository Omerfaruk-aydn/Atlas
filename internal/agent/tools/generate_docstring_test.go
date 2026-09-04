package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGenerateDocstringTool(t *testing.T, workingDir string, params GenerateDocstringParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGenerateDocstringTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GenerateDocstringToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const docstringFixture = `package a

func Foo(name string) error {
	return nil
}
`

func TestGenerateDocstringToolReportsSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", docstringFixture)

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 undocumented exported declaration(s)")
	require.Contains(t, resp.Content, "// Foo TODO: describe what Foo does.")
	require.Contains(t, resp.Content, "replace every TODO")
}

func TestGenerateDocstringToolReportsCleanCode(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\n// Foo already documented.\nfunc Foo() {}\n")

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No undocumented exported declarations found")
}

func TestGenerateDocstringToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No Go files found")
}

func TestGenerateDocstringToolFiltersBySymbol(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n\nfunc Bar() {}\n")

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{Symbol: "Bar"})
	require.Contains(t, resp.Content, "1 undocumented exported declaration(s)")
	require.Contains(t, resp.Content, "Bar")
	require.NotContains(t, resp.Content, "// Foo TODO")
}

func TestGenerateDocstringToolReportsNoMatchForSymbol(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n")

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{Symbol: "Missing"})
	require.Contains(t, resp.Content, `No undocumented exported declaration named "Missing" found.`)
}

func TestGenerateDocstringToolSkipsTestFilesByDefault(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", docstringFixture)

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestGenerateDocstringToolIncludesTestFilesWhenAsked(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a_test.go", docstringFixture)

	include := true
	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{IncludeTests: &include})
	require.Contains(t, resp.Content, "1 undocumented exported declaration(s)")
}

func TestGenerateDocstringToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", docstringFixture)

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{})
	var meta GenerateDocstringResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.FilesScanned)
}

func TestGenerateDocstringToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", docstringFixture)

	resp := runGenerateDocstringTool(t, dir, GenerateDocstringParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 undocumented exported declaration(s)")
}
