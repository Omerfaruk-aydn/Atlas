package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
	"github.com/stretchr/testify/require"
)

func runGitBranchesTool(t *testing.T, workingDir string, params GitBranchesParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitBranchesTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitBranchesToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGitBranchesToolListsBranches(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "branch", "feature")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "2 branch(es)")
	require.Contains(t, resp.Content, "main")
	require.Contains(t, resp.Content, "feature")
}

func TestGitBranchesToolMarksTheCurrentBranch(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "checkout", "-b", "feature")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{})
	require.Contains(t, resp.Content, "* feature")
}

// Merged status decides whether deleting a branch loses work, so it is
// the one thing this tool must get right.
func TestGitBranchesToolFlagsMergedAndUnmergedBranches(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "base\n")
	toolCommit(t, dir, "init")

	toolGit(t, dir, "checkout", "-b", "merged-work")
	writeDeadCodeFile(t, dir, "b.txt", "x\n")
	toolCommit(t, dir, "merged work")
	toolGit(t, dir, "checkout", "main")
	toolGit(t, dir, "merge", "--no-ff", "merged-work", "-m", "merge it")

	toolGit(t, dir, "checkout", "-b", "unmerged-work")
	writeDeadCodeFile(t, dir, "c.txt", "y\n")
	toolCommit(t, dir, "unmerged work")
	toolGit(t, dir, "checkout", "main")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{MergedBase: "main"})
	require.Contains(t, resp.Content, "merged into main")
	require.Contains(t, resp.Content, "NOT merged")

	var meta GitBranchesResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	// main and merged-work are both reachable from main -- git counts a
	// branch as merged into itself, which is correct, just easy to
	// forget when picking the expected count.
	require.Equal(t, 2, meta.Merged)
}

// The squash/rebase caveat is what stops an agent from deleting real work
// on the strength of a "NOT merged" label.
func TestGitBranchesToolWarnsAboutSquashAndRebase(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{})
	require.Contains(t, resp.Content, "squash-merged or rebased")
	require.Contains(t, resp.Content, "confirm before treating")
}

func TestGitBranchesToolFlagsStaleBranches(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolGit(t, dir, "add", "-A")
	// Backdate the commit well past the default staleness window.
	toolGit(t, dir, "-c", "user.email=test@example.com", "-c", "user.name=Test",
		"commit", "--date=2000-01-01T00:00:00", "-m", "ancient")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{StaleDays: 30})
	require.Contains(t, resp.Content, "stale (")

	var meta GitBranchesResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Stale)
}

func TestGitBranchesToolShowsUpstreamDivergence(t *testing.T) {
	origin := toolGitRepo(t)
	writeDeadCodeFile(t, origin, "a.txt", "one\n")
	toolCommit(t, origin, "init")

	clone := t.TempDir()
	_, err := gitx.Run(context.Background(), clone, "clone", origin, ".")
	require.NoError(t, err)
	toolGit(t, clone, "config", "user.email", "test@example.com")
	toolGit(t, clone, "config", "user.name", "Test")
	writeDeadCodeFile(t, clone, "b.txt", "two\n")
	toolCommit(t, clone, "local")

	resp := runGitBranchesTool(t, clone, GitBranchesParams{})
	require.Contains(t, resp.Content, "1 ahead of")
}

func TestGitBranchesToolCanIncludeRemotes(t *testing.T) {
	origin := toolGitRepo(t)
	writeDeadCodeFile(t, origin, "a.txt", "one\n")
	toolCommit(t, origin, "init")

	clone := t.TempDir()
	_, err := gitx.Run(context.Background(), clone, "clone", origin, ".")
	require.NoError(t, err)

	without := runGitBranchesTool(t, clone, GitBranchesParams{})
	require.NotContains(t, without.Content, "(remote)")

	yes := true
	with := runGitBranchesTool(t, clone, GitBranchesParams{IncludeRemote: &yes})
	require.Contains(t, with.Content, "(remote)")
}

func TestGitBranchesToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runGitBranchesTool(t, t.TempDir(), GitBranchesParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitBranchesToolReportsMetadata(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "branch", "feature")

	resp := runGitBranchesTool(t, dir, GitBranchesParams{})
	var meta GitBranchesResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Total)
	require.Equal(t, "main", meta.BaseUsed)
}
