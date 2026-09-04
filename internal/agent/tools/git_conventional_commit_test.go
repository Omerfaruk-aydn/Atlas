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

func runGitConventionalCommitTool(t *testing.T, workingDir string, params GitConventionalCommitParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitConventionalCommitTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitConventionalCommitToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGitConventionalCommitToolSuggestsDocs(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n"), 0o644))
	preCommitGit(t, dir, "add", "README.md")

	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "docs: <description>")
	require.Contains(t, resp.Content, "Confidence: high")
}

func TestGitConventionalCommitToolReportsLowConfidenceNote(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644))
	preCommitGit(t, dir, "add", "a.go")
	preCommitGit(t, dir, "commit", "-m", "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n\nfunc F() {}\n"), 0o644))
	preCommitGit(t, dir, "add", "a.go")

	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	require.Contains(t, resp.Content, "fix: <description>")
	require.Contains(t, resp.Content, "starting point, not the answer")
}

func TestGitConventionalCommitToolReportsNothingStaged(t *testing.T) {
	dir := preCommitRepo(t)
	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	require.Contains(t, resp.Content, "Nothing is staged")
}

func TestGitConventionalCommitToolReportsNotARepository(t *testing.T) {
	dir := t.TempDir()
	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitConventionalCommitToolReportsMetadata(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hi\n"), 0o644))
	preCommitGit(t, dir, "add", "README.md")

	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	var meta GitConventionalCommitResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, "docs", meta.Type)
	require.Equal(t, "high", meta.Confidence)
	require.Equal(t, 1, meta.FilesCount)
}

func TestGitConventionalCommitToolReportsScopeInPrefix(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "internal", "gitx"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "gitx", "a.go"), []byte("package gitx\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "internal", "gitx", "a_test.go"), []byte("package gitx\n"), 0o644))
	preCommitGit(t, dir, "add", "-A")

	resp := runGitConventionalCommitTool(t, dir, GitConventionalCommitParams{})
	require.Contains(t, resp.Content, "feat(gitx): <description>")
}
