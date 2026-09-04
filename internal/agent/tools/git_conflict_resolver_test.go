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

func runGitConflictResolverTool(t *testing.T, workingDir string, params GitConflictResolverParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitConflictResolverTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitConflictResolverToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

const conflictFixture = "line1\n<<<<<<< HEAD\nours line\n=======\ntheirs line\n>>>>>>> feature\nline2\n"

func TestGitConflictResolverToolReportsAConflict(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(conflictFixture), 0o644))

	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{Path: "a.go"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 conflict(s)")
	require.Contains(t, resp.Content, "ours line")
	require.Contains(t, resp.Content, "theirs line")
	require.Contains(t, resp.Content, "feature")
}

func TestGitConflictResolverToolReportsClean(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644))

	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{Path: "a.go"})
	require.Contains(t, resp.Content, "No merge-conflict markers found")
}

func TestGitConflictResolverToolRequiresPath(t *testing.T) {
	dir := t.TempDir()
	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

func TestGitConflictResolverToolReportsMissingFile(t *testing.T) {
	dir := t.TempDir()
	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{Path: "nope.go"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "no file found")
}

func TestGitConflictResolverToolReportsMetadata(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(conflictFixture), 0o644))

	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{Path: "a.go"})
	var meta GitConflictResolverResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Conflicts)
}

func TestGitConflictResolverToolTruncatesLongSides(t *testing.T) {
	dir := t.TempDir()
	content := "<<<<<<< HEAD\n1\n2\n3\n4\n5\n6\n7\n=======\ntheirs\n>>>>>>> feature\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(content), 0o644))

	resp := runGitConflictResolverTool(t, dir, GitConflictResolverParams{Path: "a.go"})
	require.Contains(t, resp.Content, "more line(s)")
}
