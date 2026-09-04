package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func runPRDescribeTool(t *testing.T, workingDir string, params PRDescribeParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewPRDescribeTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  PRDescribeToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func prToolFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := toolGitRepo(t)
	writeDeadCodeFile(t, dir, "a.txt", "base\n")
	toolCommit(t, dir, "chore: init")
	toolGit(t, dir, "checkout", "-b", "feature")
	return dir
}

func TestPRDescribeToolDraftsFromCommitsAndDiff(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "internal/auth/login.go", "package auth\n")
	toolCommit(t, dir, "feat(auth): add login, see #42")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "## Summary")
	require.Contains(t, resp.Content, "add login")
	require.Contains(t, resp.Content, "## Changes")
	require.Contains(t, resp.Content, "**Features**")
	require.Contains(t, resp.Content, "Touches: internal")
	require.Contains(t, resp.Content, "References: #42")
	require.Contains(t, resp.Content, "not posted anywhere")
}

func TestPRDescribeToolDefaultsHeadToTheCurrentBranch(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n")
	toolCommit(t, dir, "feat: something")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.Contains(t, resp.Content, "feature -> main")
}

func TestPRDescribeToolRequiresABase(t *testing.T) {
	resp := runPRDescribeTool(t, t.TempDir(), PRDescribeParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "base is required")
}

func TestPRDescribeToolRejectsIdenticalBaseAndHead(t *testing.T) {
	dir := prToolFixtureRepo(t)
	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "feature", Head: "feature"})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "nothing to compare")
}

// A missing-tests note next to a logic change is the single most useful
// caveat this tool can surface, and it must not be silently omitted.
func TestPRDescribeToolFlagsMissingTests(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n")
	toolCommit(t, dir, "feat: logic change with no tests")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.Contains(t, resp.Content, "No test files changed")

	var meta PRDescribeResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.False(t, meta.HasTests)
}

func TestPRDescribeToolAcknowledgesTestsWhenPresent(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n")
	writeDeadCodeFile(t, dir, "a_test.go", "package a\n")
	toolCommit(t, dir, "feat: with tests")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.Contains(t, resp.Content, "Includes test changes")
}

func TestPRDescribeToolReportsNoCommits(t *testing.T) {
	dir := prToolFixtureRepo(t)
	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.Contains(t, resp.Content, "No commits found")
}

func TestPRDescribeToolNeverClaimsToPostAnything(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n")
	toolCommit(t, dir, "feat: x")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	require.Contains(t, resp.Content, "not posted anywhere")
	require.Contains(t, resp.Content, "This is a draft")
}

func TestPRDescribeToolAnswersPlainlyOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	resp := runPRDescribeTool(t, t.TempDir(), PRDescribeParams{Base: "main"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "Not a git repository")
}

func TestPRDescribeToolReportsMetadata(t *testing.T) {
	dir := prToolFixtureRepo(t)
	writeDeadCodeFile(t, dir, "a.go", "package a\n")
	toolCommit(t, dir, "feat: x")
	writeDeadCodeFile(t, dir, "b.go", "package a\n")
	toolCommit(t, dir, "fix: y")

	resp := runPRDescribeTool(t, dir, PRDescribeParams{Base: "main"})
	var meta PRDescribeResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, 2, meta.Commits)
	require.Equal(t, 2, meta.FilesChanged)
}
