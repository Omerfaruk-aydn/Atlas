package gitx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func prFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "chore: init")
	git(t, dir, "checkout", "-b", "feature")
	return dir
}

func TestSummarisePRGathersCommitsAndDiff(t *testing.T) {
	dir := prFixtureRepo(t)
	writeFile(t, dir, "internal/auth/login.go", "package auth\n")
	commit(t, dir, "feat(auth): add login, see #42")
	writeFile(t, dir, "internal/auth/login_test.go", "package auth\n")
	commit(t, dir, "test(auth): cover login")

	got, err := SummarisePR(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	require.Len(t, got.Commits, 2)
	require.Len(t, got.Diff.Files, 2)
	require.True(t, got.HasTests)
	require.Contains(t, got.TicketRefs, "#42")
}

func TestSummarisePRCountsTopLevelDirectories(t *testing.T) {
	dir := prFixtureRepo(t)
	writeFile(t, dir, "internal/auth/a.go", "package auth\n")
	writeFile(t, dir, "internal/auth/b.go", "package auth\n")
	writeFile(t, dir, "internal/ui/c.go", "package ui\n")
	commit(t, dir, "feat: touch several packages")

	got, err := SummarisePR(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	require.Equal(t, 3, got.TopLevelDirs["internal"])
}

// A root-level file (README.md, go.mod) has no directory component, and
// that must not crash or be silently dropped from the tally.
func TestSummarisePRHandlesRootLevelFiles(t *testing.T) {
	dir := prFixtureRepo(t)
	writeFile(t, dir, "README.md", "docs\n")
	commit(t, dir, "docs: update readme")

	got, err := SummarisePR(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	require.Equal(t, 1, got.TopLevelDirs["."])
}

func TestSummarisePRDeduplicatesTicketReferences(t *testing.T) {
	dir := prFixtureRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	commit(t, dir, "feat: one, see #42")
	writeFile(t, dir, "b.go", "package a\n")
	commit(t, dir, "fix: two, also #42 and PROJ-7")

	got, err := SummarisePR(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"#42", "PROJ-7"}, got.TicketRefs)
}

func TestSummarisePRDetectsNoTestsChanged(t *testing.T) {
	dir := prFixtureRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	commit(t, dir, "feat: no tests here")

	got, err := SummarisePR(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	require.False(t, got.HasTests)
}

func TestSortedDirsOrdersByCountThenName(t *testing.T) {
	got := SortedDirs(map[string]int{"b": 1, "a": 1, "c": 5})
	require.Equal(t, []string{"c", "a", "b"}, got)
}

func TestSummarisePRFailsOnAnUnknownBase(t *testing.T) {
	dir := prFixtureRepo(t)
	_, err := SummarisePR(context.Background(), dir, "no-such-base", "feature")
	require.Error(t, err)
}
