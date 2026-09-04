package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ghpr"
	"github.com/stretchr/testify/require"
)

func runGithubPRViewTool(t *testing.T, workingDir string, params GithubPRViewParams) fantasy.ToolResponse {
	t.Helper()
	tool := NewGithubPRViewTool(workingDir)
	input, err := json.Marshal(params)
	require.NoError(t, err)
	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  GithubPRViewToolName,
		Input: string(input),
	})
	require.NoError(t, err)
	return resp
}

func TestGithubPRViewToolRequiresRef(t *testing.T) {
	dir := t.TempDir()
	resp := runGithubPRViewTool(t, dir, GithubPRViewParams{})
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "ref is required")
}

func TestGithubPRViewToolReportsGHMissing(t *testing.T) {
	if _, err := exec.LookPath("gh"); err == nil {
		t.Skip("gh is installed on this machine; the missing-binary path isn't reachable here")
	}
	dir := t.TempDir()
	resp := runGithubPRViewTool(t, dir, GithubPRViewParams{Ref: "42"})
	require.False(t, resp.IsError)
	require.Contains(t, resp.Content, "gh is not installed")
}

func TestGhErrorResponseMapsKnownErrors(t *testing.T) {
	resp := ghErrorResponse(ghpr.ErrGHMissing)
	require.Contains(t, resp.Content, "gh is not installed")

	resp = ghErrorResponse(ghpr.ErrNotAuthenticated)
	require.Contains(t, resp.Content, "gh auth login")

	resp = ghErrorResponse(ghpr.ErrBadRef)
	require.True(t, resp.IsError)

	resp = ghErrorResponse(errors.New("boom"))
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "boom")
}

func TestFormatPRViewIncludesMetadataAndDiff(t *testing.T) {
	out := formatPRView(ghpr.PR{
		Number: 42, Title: "Add widget", State: "OPEN",
		Author: "octocat", BaseRefName: "main", HeadRefName: "feature/widget",
		URL:       "https://github.com/octo/repo/pull/42",
		Additions: 10, Deletions: 2, ChangedFiles: 3,
		Body: "This adds a widget.",
		Diff: "diff --git a/x b/x\n",
	})
	require.Contains(t, out, "#42 Add widget [OPEN]")
	require.Contains(t, out, "feature/widget -> main, by octocat")
	require.Contains(t, out, "This adds a widget.")
	require.Contains(t, out, "diff --git")
}

func TestFormatPRViewReportsMissingDiff(t *testing.T) {
	out := formatPRView(ghpr.PR{Number: 1, Title: "x", State: "OPEN"})
	require.Contains(t, out, "diff unavailable")
}

func TestFormatPRViewTruncatesALargeDiff(t *testing.T) {
	out := formatPRView(ghpr.PR{
		Number: 1, Title: "x", State: "OPEN",
		Diff: strings.Repeat("a", maxPRDiffBytes+1000),
	})
	require.Contains(t, out, "truncated")
}
