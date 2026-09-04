package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGitCommitSplitTool(t *testing.T, workingDir string, params GitCommitSplitParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitCommitSplitTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitCommitSplitToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGitCommitSplitToolReportsGroups(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeDeadCodeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	preCommitGit(t, dir, "add", "-A")
	preCommitGit(t, dir, "commit", "-m", "init")

	writeDeadCodeFile(t, dir, "a/a.go", "package a\n\nfunc A() int { return 1 }\n")

	resp := runGitCommitSplitTool(t, dir, GitCommitSplitParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 commit(s) suggested")
	require.Contains(t, resp.Content, "example.com/app/a")
	require.Contains(t, resp.Content, "Nothing was staged or committed")
}

func TestGitCommitSplitToolReportsNothingChanged(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "hello\n")
	preCommitGit(t, dir, "add", "-A")
	preCommitGit(t, dir, "commit", "-m", "init")

	resp := runGitCommitSplitTool(t, dir, GitCommitSplitParams{})
	require.Contains(t, resp.Content, "Nothing has changed")
}

func TestGitCommitSplitToolReportsNotARepository(t *testing.T) {
	dir := t.TempDir()
	resp := runGitCommitSplitTool(t, dir, GitCommitSplitParams{})
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitCommitSplitToolReportsMetadata(t *testing.T) {
	dir := preCommitRepo(t)
	writeDeadCodeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeDeadCodeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	preCommitGit(t, dir, "add", "-A")
	preCommitGit(t, dir, "commit", "-m", "init")

	writeDeadCodeFile(t, dir, "a/a.go", "package a\n\nfunc A() int { return 1 }\n")

	resp := runGitCommitSplitTool(t, dir, GitCommitSplitParams{})
	var meta GitCommitSplitResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Groups)
	require.Equal(t, 0, meta.Cycles)
}
