package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGitDiffTool(t *testing.T, workingDir string, params GitDiffParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitDiffTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitDiffToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGitDiffToolSummarisesWorkingTreeChanges(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "one\ntwo\n")

	resp := runGitDiffTool(t, dir, GitDiffParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "1 file(s) changed")
	require.Contains(t, resp.Content, "a.txt")
	require.Contains(t, resp.Content, "+1")
}

// "Nothing changed" gets misread as "my edit did not land" unless the
// output says which comparison produced it -- the edit may just be
// staged.
func TestGitDiffToolNamesWhatItCompared(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")

	resp := runGitDiffTool(t, dir, GitDiffParams{})
	require.Contains(t, resp.Content, "working tree vs. the index")

	yes := true
	resp = runGitDiffTool(t, dir, GitDiffParams{Staged: &yes})
	require.Contains(t, resp.Content, "index vs. HEAD")
	require.Contains(t, resp.Content, "what a commit would capture")
}

func TestGitDiffToolSeesStagedChanges(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "one\nstaged\n")
	toolGit(t, dir, "add", "a.txt")

	yes := true
	resp := runGitDiffTool(t, dir, GitDiffParams{Staged: &yes})
	require.Contains(t, resp.Content, "1 file(s) changed")

	// Unstaged is now empty, and must say so without implying the edit
	// was lost.
	resp = runGitDiffTool(t, dir, GitDiffParams{})
	require.Contains(t, resp.Content, "No changes in the working tree vs. the index")
}

// --cached with a ref means index-vs-that-ref, which is neither thing the
// caller meant.
func TestGitDiffToolRejectsStagedTogetherWithRef(t *testing.T) {
	yes := true
	resp := runGitDiffTool(t, t.TempDir(), GitDiffParams{Staged: &yes, Ref: "main"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "not both")
}

func TestGitDiffToolComparesARange(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "base\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "checkout", "-b", "feature")
	writeDeadCodeFile(t, dir, "b.txt", "feature\n")
	toolCommit(t, dir, "feature work")

	resp := runGitDiffTool(t, dir, GitDiffParams{Ref: "main..feature"})
	require.Contains(t, resp.Content, "across main..feature")
	require.Contains(t, resp.Content, "b.txt")
}

func TestGitDiffToolOmitsPatchByDefault(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "changed\n")

	resp := runGitDiffTool(t, dir, GitDiffParams{})
	require.NotContains(t, resp.Content, "@@")
}

func TestGitDiffToolIncludesPatchWhenAsked(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "changed\n")

	yes := true
	resp := runGitDiffTool(t, dir, GitDiffParams{WithPatch: &yes})
	require.Contains(t, resp.Content, "--- a.txt ---")
	require.Contains(t, resp.Content, "-one")
	require.Contains(t, resp.Content, "+changed")
}

func TestGitDiffToolNarrowsToAPath(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	writeDeadCodeFile(t, dir, "b.txt", "one\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "changed\n")
	writeDeadCodeFile(t, dir, "b.txt", "changed\n")

	resp := runGitDiffTool(t, dir, GitDiffParams{Path: "a.txt"})
	require.Contains(t, resp.Content, "under a.txt")
	require.Contains(t, resp.Content, "a.txt")
	require.NotContains(t, resp.Content, "b.txt")
}

// A rename shows a path that does not exist unless it is expanded, and
// every subsequent read of that path then fails.
func TestGitDiffToolShowsBothSidesOfARename(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "old.txt", "content long enough to be detected as a rename\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "mv", "old.txt", "new.txt")

	yes := true
	resp := runGitDiffTool(t, dir, GitDiffParams{Staged: &yes})
	require.Contains(t, resp.Content, "new.txt (renamed from old.txt)")
}

func TestGitDiffToolReportsACleanTree(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "init")

	resp := runGitDiffTool(t, dir, GitDiffParams{})
	require.Contains(t, resp.Content, "No changes")
}

func TestGitDiffToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runGitDiffTool(t, t.TempDir(), GitDiffParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitDiffToolReportsMetadata(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\ntwo\n")
	toolCommit(t, dir, "init")
	writeDeadCodeFile(t, dir, "a.txt", "one\n")

	resp := runGitDiffTool(t, dir, GitDiffParams{})
	var meta GitDiffResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.Files)
	require.Equal(t, 1, meta.Deletions)
	require.Zero(t, meta.Insertions)
}
