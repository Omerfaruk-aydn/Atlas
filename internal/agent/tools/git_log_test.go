package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runGitLogTool(t *testing.T, workingDir string, params GitLogParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGitLogTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GitLogToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func logFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "first: add a")
	writeDeadCodeFile(t, dir, "b.txt", "two\n")
	toolCommit(t, dir, "second: add b")
	return dir
}

func TestGitLogToolListsCommitsNewestFirst(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "2 commit(s)")
	require.Less(t,
		indexOf(resp.Content, "second: add b"),
		indexOf(resp.Content, "first: add a"))
}

func TestGitLogToolFiltersByPath(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{Path: "b.txt"})
	require.Contains(t, resp.Content, "second: add b")
	require.NotContains(t, resp.Content, "first: add a")
	// The filter has to be restated, or a short result reads as "this
	// file has barely changed".
	require.Contains(t, resp.Content, "touching b.txt")
}

func TestGitLogToolRestatesEveryFilter(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{
		Path:   "nothing-matches-this",
		Author: "nobody",
		Grep:   "xyzzy",
		Since:  "1 year ago",
	})
	require.Contains(t, resp.Content, "No commits matched")
	require.Contains(t, resp.Content, "touching nothing-matches-this")
	require.Contains(t, resp.Content, "by nobody")
	require.Contains(t, resp.Content, "mentioning xyzzy")
	require.Contains(t, resp.Content, "since 1 year ago")
}

func TestGitLogToolFiltersByMessage(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{Grep: "add b"})
	require.Contains(t, resp.Content, "second: add b")
	require.NotContains(t, resp.Content, "first: add a")
}

func TestGitLogToolHonoursTheLimit(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{Limit: 1})
	require.Contains(t, resp.Content, "1 commit(s)")
	require.NotContains(t, resp.Content, "first: add a")
}

func TestGitLogToolClampsAnAbsurdLimit(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{Limit: 999999})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "2 commit(s)")
}

func TestGitLogToolShowsStatsWhenAsked(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	toolCommit(t, dir, "add three lines")

	yes := true
	resp := runGitLogTool(t, dir, GitLogParams{WithStats: &yes})
	require.Contains(t, resp.Content, "1 file(s), +3 -0")
	require.Contains(t, resp.Content, "a.txt")
}

func TestGitLogToolOmitsStatsByDefault(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{})
	require.NotContains(t, resp.Content, "file(s), +")
}

func TestGitLogToolShowsTheCommitBody(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "x\n")
	toolGit(t, dir, "add", "-A")
	toolGit(t, dir, "commit", "-m", "subject", "-m", "why this was done")

	resp := runGitLogTool(t, dir, GitLogParams{})
	require.Contains(t, resp.Content, "| why this was done")
}

func TestGitLogToolMarksMergeCommits(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "base\n")
	toolCommit(t, dir, "init")
	toolGit(t, dir, "checkout", "-b", "feature")
	writeDeadCodeFile(t, dir, "b.txt", "feature\n")
	toolCommit(t, dir, "feature work")
	toolGit(t, dir, "checkout", "main")
	toolGit(t, dir, "merge", "--no-ff", "feature", "-m", "merge feature")

	resp := runGitLogTool(t, dir, GitLogParams{})
	require.Contains(t, resp.Content, "[merge]")

	no := true
	resp = runGitLogTool(t, dir, GitLogParams{NoMerges: &no})
	require.NotContains(t, resp.Content, "[merge]")
	require.Contains(t, resp.Content, "excluding merges")
}

func TestGitLogToolShowsRelativeAge(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{})
	// The absolute date is there for precision, and the relative age
	// beside it is what a reader actually orients on. These commits were
	// made a moment ago, so that is what it must say.
	require.Contains(t, resp.Content, time.Now().Format("2006-01-02"))
	require.Contains(t, resp.Content, "(just now)")
}

func TestGitLogToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runGitLogTool(t, t.TempDir(), GitLogParams{})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestGitLogToolReportsMetadata(t *testing.T) {
	dir := logFixtureRepo(t)

	resp := runGitLogTool(t, dir, GitLogParams{})
	var meta GitLogResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Commits)
	require.NotEmpty(t, meta.Newest)
	require.NotEmpty(t, meta.Oldest)
	require.NotEqual(t, meta.Newest, meta.Oldest)
}

func TestRelativeAgeCoversEveryScale(t *testing.T) {
	now := time.Now()
	require.Equal(t, "just now", relativeAge(now))
	require.Equal(t, "5m ago", relativeAge(now.Add(-5*time.Minute)))
	require.Equal(t, "3h ago", relativeAge(now.Add(-3*time.Hour)))
	require.Equal(t, "2d ago", relativeAge(now.Add(-48*time.Hour)))
	require.Equal(t, "2mo ago", relativeAge(now.Add(-60*24*time.Hour)))
	require.Equal(t, "2y ago", relativeAge(now.Add(-2*365*24*time.Hour)))
}
