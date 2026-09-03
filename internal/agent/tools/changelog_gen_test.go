package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runChangelogTool(t *testing.T, workingDir string, params ChangelogGenParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewChangelogGenTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  ChangelogGenToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func changelogFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "chore: init")
	writeDeadCodeFile(t, dir, "b.txt", "two\n")
	toolCommit(t, dir, "feat(auth): add SSO login")
	writeDeadCodeFile(t, dir, "c.txt", "three\n")
	toolCommit(t, dir, "fix: correct off-by-one")
	return dir
}

func TestChangelogToolGroupsCommitsIntoSections(t *testing.T) {
	dir := changelogFixtureRepo(t)

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "## Features")
	require.Contains(t, resp.Content, "add SSO login")
	require.Contains(t, resp.Content, "## Fixes")
	require.Contains(t, resp.Content, "correct off-by-one")
	require.Contains(t, resp.Content, "## Chores")
}

func TestChangelogToolShowsTheScope(t *testing.T) {
	dir := changelogFixtureRepo(t)

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.Contains(t, resp.Content, "**auth:** add SSO login")
}

func TestChangelogToolRequiresARange(t *testing.T) {
	resp := runChangelogTool(t, t.TempDir(), ChangelogGenParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "range is required")
}

func TestChangelogToolHonoursARevisionRange(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "chore: init")
	toolGit(t, dir, "tag", "v1.0.0")
	writeDeadCodeFile(t, dir, "b.txt", "two\n")
	toolCommit(t, dir, "feat: new since tag")

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "v1.0.0..HEAD"})
	require.Contains(t, resp.Content, "new since tag")
	require.NotContains(t, resp.Content, "chore")
}

// A breaking feat must appear in both sections: it is still a feature,
// and hiding it from the features list would make it invisible there.
func TestChangelogToolListsBreakingChangesSeparatelyAndInTheirType(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "feat(api)!: remove legacy endpoint")

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.Contains(t, resp.Content, "## BREAKING CHANGES")
	require.Contains(t, resp.Content, "## Features")

	var meta ChangelogGenResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 1, meta.BreakingChanges)
}

// A non-conventional commit silently dropped makes the changelog look
// complete when it is not.
func TestChangelogToolKeepsNonConventionalCommits(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "quick hotfix without any convention")

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.Contains(t, resp.Content, "## Other")
	require.Contains(t, resp.Content, "quick hotfix without any convention")
	require.Contains(t, resp.Content, "did not follow Conventional Commits")
}

func TestChangelogToolReportsNoCommitsInRange(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "one\n")
	toolCommit(t, dir, "chore: init")
	toolGit(t, dir, "tag", "v1.0.0")

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "v1.0.0..HEAD"})
	require.Contains(t, resp.Content, "No commits found")
}

func TestChangelogToolSaysItIsADraft(t *testing.T) {
	dir := changelogFixtureRepo(t)

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.Contains(t, resp.Content, "not saved to any file")
}

func TestChangelogToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runChangelogTool(t, t.TempDir(), ChangelogGenParams{Range: "HEAD"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestChangelogToolShowsBusiestScopes(t *testing.T) {
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "1\n")
	toolCommit(t, dir, "feat(auth): one")
	writeDeadCodeFile(t, dir, "b.txt", "2\n")
	toolCommit(t, dir, "feat(auth): two")
	writeDeadCodeFile(t, dir, "c.txt", "3\n")
	toolCommit(t, dir, "fix(ui): three")

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	require.Contains(t, resp.Content, "Busiest scopes:")
	require.Contains(t, resp.Content, "auth (2)")
}

func TestChangelogToolReportsMetadata(t *testing.T) {
	dir := changelogFixtureRepo(t)

	resp := runChangelogTool(t, dir, ChangelogGenParams{Range: "HEAD"})
	var meta ChangelogGenResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 3, meta.Commits)
	require.Equal(t, 3, meta.Conventional)
	require.Equal(t, 0, meta.NonConventional)
}
