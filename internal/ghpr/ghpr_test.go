package ghpr

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseRefBareNumber(t *testing.T) {
	owner, repo, number, err := parseRef("42")
	require.NoError(t, err)
	require.Empty(t, owner)
	require.Empty(t, repo)
	require.Equal(t, "42", number)
}

func TestParseRefBareNumberWithHash(t *testing.T) {
	_, _, number, err := parseRef("#42")
	require.NoError(t, err)
	require.Equal(t, "42", number)
}

func TestParseRefOwnerRepoHashNumber(t *testing.T) {
	owner, repo, number, err := parseRef("golang/go#12345")
	require.NoError(t, err)
	require.Equal(t, "golang", owner)
	require.Equal(t, "go", repo)
	require.Equal(t, "12345", number)
}

func TestParseRefFullURL(t *testing.T) {
	owner, repo, number, err := parseRef("https://github.com/golang/go/pull/12345")
	require.NoError(t, err)
	require.Equal(t, "golang", owner)
	require.Equal(t, "go", repo)
	require.Equal(t, "12345", number)
}

func TestParseRefFullURLWithGitSuffix(t *testing.T) {
	owner, repo, _, err := parseRef("https://github.com/golang/go.git/pull/12345")
	require.NoError(t, err)
	require.Equal(t, "go", repo)
	_ = owner
}

func TestParseRefRejectsGarbage(t *testing.T) {
	_, _, _, err := parseRef("not a pr reference at all")
	require.ErrorIs(t, err, ErrBadRef)
}

func fakeRunner(t *testing.T, calls *[][]string, viewJSON, diffText string, diffErr error) Runner {
	t.Helper()
	return func(ctx context.Context, dir string, args ...string) (string, error) {
		*calls = append(*calls, args)
		if len(args) >= 3 && args[2] == "view" {
			return viewJSON, nil
		}
		if len(args) >= 3 && args[2] == "diff" {
			if diffErr != nil {
				return "", diffErr
			}
			return diffText, nil
		}
		return "", errors.New("unexpected args: " + strings.Join(args, " "))
	}
}

const samplePRJSON = `{
	"number": 42,
	"title": "Add the widget",
	"body": "This adds a widget.",
	"author": {"login": "octocat"},
	"state": "OPEN",
	"baseRefName": "main",
	"headRefName": "feature/widget",
	"url": "https://github.com/octo/repo/pull/42",
	"additions": 10,
	"deletions": 2,
	"changedFiles": 3
}`

func TestViewParsesMetadataAndDiff(t *testing.T) {
	var calls [][]string
	c := &Client{run: fakeRunner(t, &calls, samplePRJSON, "diff --git a/x b/x\n", nil)}

	pr, err := c.View(context.Background(), "/repo", "42")
	require.NoError(t, err)
	require.Equal(t, 42, pr.Number)
	require.Equal(t, "Add the widget", pr.Title)
	require.Equal(t, "octocat", pr.Author)
	require.Equal(t, "main", pr.BaseRefName)
	require.Equal(t, "feature/widget", pr.HeadRefName)
	require.Equal(t, 10, pr.Additions)
	require.Contains(t, pr.Diff, "diff --git")
}

func TestViewBuildsRepoFlagFromOwnerRepoRef(t *testing.T) {
	var calls [][]string
	c := &Client{run: fakeRunner(t, &calls, samplePRJSON, "", nil)}

	_, err := c.View(context.Background(), "/repo", "octo/repo#42")
	require.NoError(t, err)
	require.Contains(t, calls[0], "-R")
	require.Contains(t, calls[0], "octo/repo")
}

func TestViewOmitsRepoFlagForABareNumber(t *testing.T) {
	var calls [][]string
	c := &Client{run: fakeRunner(t, &calls, samplePRJSON, "", nil)}

	_, err := c.View(context.Background(), "/repo", "42")
	require.NoError(t, err)
	require.NotContains(t, calls[0], "-R")
}

func TestViewToleratesADiffFailure(t *testing.T) {
	var calls [][]string
	c := &Client{run: fakeRunner(t, &calls, samplePRJSON, "", errors.New("diff too large"))}

	pr, err := c.View(context.Background(), "/repo", "42")
	require.NoError(t, err)
	require.Empty(t, pr.Diff)
}

func TestViewRejectsABadRef(t *testing.T) {
	c := &Client{run: func(ctx context.Context, dir string, args ...string) (string, error) {
		t.Fatal("run should not be called for a bad ref")
		return "", nil
	}}

	_, err := c.View(context.Background(), "/repo", "not a ref")
	require.ErrorIs(t, err, ErrBadRef)
}

func TestViewPropagatesAViewFailure(t *testing.T) {
	c := &Client{run: func(ctx context.Context, dir string, args ...string) (string, error) {
		return "", ErrGHMissing
	}}

	_, err := c.View(context.Background(), "/repo", "42")
	require.ErrorIs(t, err, ErrGHMissing)
}

func TestViewReportsBadJSON(t *testing.T) {
	c := &Client{run: func(ctx context.Context, dir string, args ...string) (string, error) {
		return "not json", nil
	}}

	_, err := c.View(context.Background(), "/repo", "42")
	require.Error(t, err)
}

func TestNewClientReportsGHMissing(t *testing.T) {
	if _, err := exec.LookPath("gh"); err == nil {
		t.Skip("gh is installed on this machine; the missing-binary path isn't reachable here")
	}
	c := New()
	_, err := c.View(context.Background(), t.TempDir(), "42")
	require.ErrorIs(t, err, ErrGHMissing)
}
