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

func writeDocIndexFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func runDocIndexTool(t *testing.T, workingDir string, params DocIndexParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewDocIndexTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  DocIndexToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestDocIndexToolReportsHeadings(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "a.md", "# Guide\n\n## Installation\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 Markdown file(s)")
	require.Contains(t, resp.Content, "Guide")
	require.Contains(t, resp.Content, "Installation")
}

func TestDocIndexToolReportsNoFiles(t *testing.T) {
	dir := t.TempDir()
	resp := runDocIndexTool(t, dir, DocIndexParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No Markdown files found")
}

func TestDocIndexToolFiltersByQuery(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "a.md", "# Installation\n")
	writeDocIndexFile(t, dir, "b.md", "# Unrelated\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{Query: "install"})
	require.Contains(t, resp.Content, "1 of 2 Markdown file(s) match")
	require.Contains(t, resp.Content, "Installation")
	require.NotContains(t, resp.Content, "Unrelated")
}

func TestDocIndexToolReportsNoQueryMatch(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "a.md", "# Something\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{Query: "missing"})
	require.Contains(t, resp.Content, `No file's title or headings matched "missing"`)
}

func TestDocIndexToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "a.md", "# Guide\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{})
	var meta DocIndexResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.FilesMatched)
	require.Equal(t, 1, meta.FilesScanned)
}

func TestDocIndexToolRespectsMaxDepth(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "a.md", "# Title\n\n## Section\n\n### Detail\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{MaxDepth: 1})
	require.Contains(t, resp.Content, "Title")
	require.NotContains(t, resp.Content, "Section")
}

func TestDocIndexToolResolvesARelativePath(t *testing.T) {
	dir := t.TempDir()
	writeDocIndexFile(t, dir, "sub/a.md", "# Title\n")

	resp := runDocIndexTool(t, dir, DocIndexParams{Path: "sub"})
	require.Contains(t, resp.Content, "1 Markdown file(s)")
}
