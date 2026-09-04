package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func preCommitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
	return dir
}

func preCommitGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
}

func runPreCommitGuardTool(t *testing.T, workingDir string, params PreCommitGuardParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewPreCommitGuardTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  PreCommitGuardToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestPreCommitGuardToolReportsAMergeConflictMarker(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> b\n"), 0o644))
	preCommitGit(t, dir, "add", "a.txt")

	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "merge-conflict-marker")
	require.Contains(t, resp.Content, "Review each before committing")
}

func TestPreCommitGuardToolReportsClean(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello\n"), 0o644))
	preCommitGit(t, dir, "add", "a.txt")

	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "No issues found")
}

func TestPreCommitGuardToolReportsNothingStaged(t *testing.T) {
	dir := preCommitRepo(t)
	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Nothing is staged")
}

func TestPreCommitGuardToolReportsNotARepository(t *testing.T) {
	dir := t.TempDir()
	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{})
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestPreCommitGuardToolReportsMetadata(t *testing.T) {
	dir := preCommitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("<<<<<<< HEAD\n"), 0o644))
	preCommitGit(t, dir, "add", "a.txt")

	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{})
	var meta PreCommitGuardResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Total)
	require.Equal(t, 1, meta.FilesStaged)
}

func TestPreCommitGuardToolRespectsMaxFileBytes(t *testing.T) {
	dir := preCommitRepo(t)
	content := make([]byte, 2048)
	for i := range content {
		content[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), content, 0o644))
	preCommitGit(t, dir, "add", "big.bin")

	resp := runPreCommitGuardTool(t, dir, PreCommitGuardParams{MaxFileBytes: 1024})
	require.Contains(t, resp.Content, "large-file")
}
