package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGitBlameTool(t *testing.T, workingDir string, params GitBlameParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitBlameTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitBlameToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func blameFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "f.txt", "line one\nline two\n")
	toolCommit(t, dir, "first commit")

	writeDeadCodeFile(t, dir, "f.txt", "line one\nline two\nline three\nline four\n")
	toolGit(t, dir, "add", "-A")
	toolGit(t, dir, "-c", "user.name=Second", "-c", "user.email=second@example.com",
		"commit", "-m", "second commit")
	return dir
}

// Blame is read as blocks, not as a per-line list, so consecutive lines
// from one commit must arrive as one entry.
func TestGitBlameToolGroupsLinesIntoBlocks(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "4 line(s) from 2 commit(s)")
	require.Contains(t, resp.Content, "lines 1-2 (2)")
	require.Contains(t, resp.Content, "lines 3-4 (2)")
	require.Contains(t, resp.Content, "first commit")
	require.Contains(t, resp.Content, "second commit")
}

func TestGitBlameToolSummarisesOwnership(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	require.Contains(t, resp.Content, "ownership:")
	require.Contains(t, resp.Content, "Test")
	require.Contains(t, resp.Content, "Second")
	require.Contains(t, resp.Content, "50%")
}

func TestGitBlameToolRequiresAPath(t *testing.T) {
	resp := runGitBlameTool(t, t.TempDir(), GitBlameParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "path is required")
}

// An end_line with no start_line would blame line 0, return nothing, and
// look exactly like an empty file.
func TestGitBlameToolRejectsAnEndLineWithoutAStart(t *testing.T) {
	resp := runGitBlameTool(t, t.TempDir(), GitBlameParams{Path: "f.txt", EndLine: 10})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "end_line needs a start_line")
}

func TestGitBlameToolRejectsAnInvertedRange(t *testing.T) {
	resp := runGitBlameTool(t, t.TempDir(), GitBlameParams{Path: "f.txt", StartLine: 10, EndLine: 3})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "before start_line")
}

func TestGitBlameToolHonoursALineRange(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt", StartLine: 3, EndLine: 4})
	require.Contains(t, resp.Content, "2 line(s) from 1 commit(s)")
	require.Contains(t, resp.Content, "second commit")
	require.NotContains(t, resp.Content, "first commit")
}

func TestGitBlameToolShowsSourceWhenAsked(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	require.NotContains(t, resp.Content, "line three")

	yes := true
	resp = runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt", ShowLines: &yes})
	require.Contains(t, resp.Content, "line three")
}

func TestGitBlameToolCanIgnoreWhitespace(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "f.txt", "original line\n")
	toolCommit(t, dir, "wrote the line")
	writeDeadCodeFile(t, dir, "f.txt", "    original line\n")
	toolGit(t, dir, "add", "-A")
	toolGit(t, dir, "-c", "user.name=Reformatter", "-c", "user.email=r@example.com",
		"commit", "-m", "reindent")

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	require.Contains(t, resp.Content, "Reformatter")

	yes := true
	resp = runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt", IgnoreWhitespace: &yes})
	require.Contains(t, resp.Content, "wrote the line")
}

func TestGitBlameToolReadsAnOlderRevision(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt", Rev: "HEAD~1"})
	require.Contains(t, resp.Content, "2 line(s) from 1 commit(s)")
}

// Treating the newest commit on a line as the origin of its logic is the
// standard misreading of blame, so the caveat has to be in the output.
func TestGitBlameToolWarnsThatAttributionCanBeReset(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	require.Contains(t, resp.Content, "not always the one that introduced the logic")
	require.Contains(t, resp.Content, "rename")
}

func TestGitBlameToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runGitBlameTool(t, t.TempDir(), GitBlameParams{Path: "f.txt"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitBlameToolReportsMetadata(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "f.txt"})
	var meta GitBlameResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 4, meta.Lines)
	require.Equal(t, 2, meta.Spans)
	require.Equal(t, 2, meta.Authors)
}

func TestGitBlameToolErrorsOnAMissingFile(t *testing.T) {
	dir := blameFixtureRepo(t)

	resp := runGitBlameTool(t, dir, GitBlameParams{Path: "no-such-file.txt"})
	require.True(t, resp.IsError)
}
