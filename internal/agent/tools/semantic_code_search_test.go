package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runSemanticCodeSearchTool(t *testing.T, workingDir string, params SemanticCodeSearchParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewSemanticCodeSearchTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  SemanticCodeSearchToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const semanticSearchFixture = "package a\n\nfunc ParseWidget() {}\n"

func TestSemanticCodeSearchToolReportsMatches(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", semanticSearchFixture)

	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Query: "parse widget"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 match(es)")
	require.Contains(t, resp.Content, "ParseWidget")
	require.Contains(t, resp.Content, "matched: parse, widget")
}

func TestSemanticCodeSearchToolReportsNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", semanticSearchFixture)

	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Query: "completely unrelated term"})
	require.Contains(t, resp.Content, "No symbol's name or doc comment shares vocabulary")
}

func TestSemanticCodeSearchToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Query: "widget"})
	require.Contains(t, resp.Content, "No Go files found")
}

func TestSemanticCodeSearchToolRequiresQuery(t *testing.T) {
	dir := t.TempDir()
	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "query is required")
}

func TestSemanticCodeSearchToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", semanticSearchFixture)

	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Query: "widget"})
	var meta SemanticCodeSearchResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Matches)
	require.Equal(t, 1, meta.FilesScanned)
}

func TestSemanticCodeSearchToolRespectsLimit(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "a.go", "package a\n\nfunc Widget1() {}\nfunc Widget2() {}\n")

	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Query: "widget", Limit: 1})
	require.Contains(t, resp.Content, "1 match(es)")
}

func TestSemanticCodeSearchToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDeadCodeFile(t, dir, "sub/a.go", semanticSearchFixture)

	resp := runSemanticCodeSearchTool(t, dir, SemanticCodeSearchParams{Path: "sub", Query: "widget"})
	require.Contains(t, resp.Content, "1 match(es)")
}
