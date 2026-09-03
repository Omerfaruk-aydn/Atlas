package gitx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func subjects(commits []Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.Subject)
	}
	return out
}

// historyRepo builds three commits with distinguishable authors, paths
// and messages, so every filter can be checked against it.
func historyRepo(t *testing.T) string {
	t.Helper()
	dir := gitRepo(t)

	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "first: add a")

	writeFile(t, dir, "b.txt", "two\n")
	git(t, dir, "add", "-A")
	git(t, dir, "-c", "user.name=Other", "-c", "user.email=other@example.com",
		"commit", "-m", "second: add b")

	writeFile(t, dir, "a.txt", "one changed\n")
	commit(t, dir, "third: change a")

	return dir
}

func TestLogReturnsNewestFirst(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{})
	require.NoError(t, err)
	require.Equal(t,
		[]string{"third: change a", "second: add b", "first: add a"},
		subjects(got))
}

func TestLogHonoursTheLimit(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{Limit: 2})
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "third: change a", got[0].Subject)
}

func TestLogFiltersByPath(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{Path: "b.txt"})
	require.NoError(t, err)
	require.Equal(t, []string{"second: add b"}, subjects(got))
}

func TestLogFiltersByAuthor(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{Author: "other@example.com"})
	require.NoError(t, err)
	require.Equal(t, []string{"second: add b"}, subjects(got))
}

func TestLogFiltersByMessage(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{Grep: "add b"})
	require.NoError(t, err)
	require.Equal(t, []string{"second: add b"}, subjects(got))
}

// A user searching for "fix(auth)" means that text, not a regexp whose
// parentheses are a capture group.
func TestLogTreatsGrepAsALiteralString(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	commit(t, dir, "fix(auth): tighten check")
	writeFile(t, dir, "b.txt", "y\n")
	commit(t, dir, "fixauth: unrelated")

	got, err := Log(context.Background(), dir, LogOptions{Grep: "fix(auth)"})
	require.NoError(t, err)
	require.Equal(t, []string{"fix(auth): tighten check"}, subjects(got))
}

// A commit body spans lines. Splitting records on newlines would truncate
// every message with a body, which is most real ones.
func TestLogPreservesAMultiLineBody(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", "subject line", "-m", "body line one\nbody line two")

	got, err := Log(context.Background(), dir, LogOptions{})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "subject line", got[0].Subject)
	require.Contains(t, got[0].Body, "body line one")
	require.Contains(t, got[0].Body, "body line two")
}

func TestLogPopulatesStatsWhenAsked(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\ntwo\nthree\n")
	commit(t, dir, "add three lines")

	got, err := Log(context.Background(), dir, LogOptions{WithStats: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, []string{"a.txt"}, got[0].Files)
	require.Equal(t, 3, got[0].Insertions)
	require.Zero(t, got[0].Deletions)
}

func TestLogLeavesStatsEmptyByDefault(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{})
	require.NoError(t, err)
	require.Empty(t, got[0].Files)
}

func TestLogIdentifiesMergeCommits(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "init")

	git(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "b.txt", "feature\n")
	commit(t, dir, "feature work")

	git(t, dir, "checkout", "main")
	writeFile(t, dir, "c.txt", "main\n")
	commit(t, dir, "main work")

	git(t, dir, "merge", "--no-ff", "feature", "-m", "merge feature")

	got, err := Log(context.Background(), dir, LogOptions{})
	require.NoError(t, err)
	require.True(t, got[0].Merge())
	require.Len(t, got[0].Parents, 2)
}

func TestLogCanDropMerges(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "init")
	git(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "b.txt", "feature\n")
	commit(t, dir, "feature work")
	git(t, dir, "checkout", "main")
	git(t, dir, "merge", "--no-ff", "feature", "-m", "merge feature")

	got, err := Log(context.Background(), dir, LogOptions{NoMerges: true})
	require.NoError(t, err)
	require.NotContains(t, subjects(got), "merge feature")
}

func TestLogFiltersByRef(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "on main")
	git(t, dir, "checkout", "-b", "side")
	writeFile(t, dir, "b.txt", "two\n")
	commit(t, dir, "on side")
	git(t, dir, "checkout", "main")

	got, err := Log(context.Background(), dir, LogOptions{Ref: "side"})
	require.NoError(t, err)
	require.Contains(t, subjects(got), "on side")

	got, err = Log(context.Background(), dir, LogOptions{})
	require.NoError(t, err)
	require.NotContains(t, subjects(got), "on side")
}

func TestLogRecordsAuthorAndDate(t *testing.T) {
	dir := historyRepo(t)

	got, err := Log(context.Background(), dir, LogOptions{Limit: 1})
	require.NoError(t, err)
	require.Equal(t, "Test", got[0].Author)
	require.Equal(t, "test@example.com", got[0].Email)
	require.False(t, got[0].Date.IsZero())
	require.NotEmpty(t, got[0].Short)
	require.Len(t, got[0].Hash, 40)
}

func TestLogOnAnEmptyRepositoryFailsCleanly(t *testing.T) {
	dir := gitRepo(t)

	// git log on a repository with no commits exits non-zero.
	_, err := Log(context.Background(), dir, LogOptions{})
	require.Error(t, err)
}

func TestShowReturnsOneCommitWithStats(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\ntwo\n")
	commit(t, dir, "the commit")

	got, err := Show(context.Background(), dir, "HEAD")
	require.NoError(t, err)
	require.Equal(t, "the commit", got.Subject)
	require.Equal(t, []string{"a.txt"}, got.Files)
	require.Equal(t, 2, got.Insertions)
}

func TestShowDefaultsToHead(t *testing.T) {
	dir := historyRepo(t)

	got, err := Show(context.Background(), dir, "")
	require.NoError(t, err)
	require.Equal(t, "third: change a", got.Subject)
}

func TestShowFailsOnAnUnknownRef(t *testing.T) {
	dir := historyRepo(t)

	_, err := Show(context.Background(), dir, "no-such-ref")
	require.Error(t, err)
}

// A path that shares its name with a branch must be read as a path.
func TestLogSeparatesPathsFromRevisions(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "main", "a file named like a branch\n")
	commit(t, dir, "add file named main")

	got, err := Log(context.Background(), dir, LogOptions{Path: "main"})
	require.NoError(t, err)
	require.Equal(t, []string{"add file named main"}, subjects(got))
}
