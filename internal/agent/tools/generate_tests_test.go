package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGenerateTestsTool(t *testing.T, workingDir string, params GenerateTestsParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGenerateTestsTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GenerateTestsToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGenerateTestsToolReportsSkeleton(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Double(n int) int { return n * 2 }\n")

	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{Symbol: "Double"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "func TestDouble(t *testing.T) {")
	require.Contains(t, resp.Content, "one placeholder case")
}

func TestGenerateTestsToolRequiresSymbol(t *testing.T) {
	dir := t.TempDir()
	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "symbol is required")
}

func TestGenerateTestsToolReportsNotFound(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo() {}\n")

	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{Symbol: "Missing"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no package-level function")
}

func TestGenerateTestsToolRejectsAMethod(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\ntype C struct{}\n\nfunc (c *C) Do() error { return nil }\n")

	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{Symbol: "Do"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "method")
}

func TestGenerateTestsToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Foo(s string) error { return nil }\n")

	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{Symbol: "Foo"})
	var meta GenerateTestsResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, "Foo", meta.FuncName)
	require.Contains(t, meta.Imports, "github.com/stretchr/testify/require")
}

func TestGenerateTestsToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", "package a\n\nfunc Foo() {}\n")

	resp := runGenerateTestsTool(t, dir, GenerateTestsParams{Path: "sub", Symbol: "Foo"})
	require.Contains(t, resp.Content, "func TestFoo(t *testing.T) {")
}
