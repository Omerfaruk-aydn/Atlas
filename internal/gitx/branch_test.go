package gitx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func branchNamed(t *testing.T, branches []Branch, name string) Branch {
	t.Helper()
	for _, b := range branches {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("no branch %q (have %d)", name, len(branches))
	return Branch{}
}

func branchNames(branches []Branch) []string {
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.Name)
	}
	return out
}

func TestBranchesListsLocalBranches(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	git(t, dir, "branch", "feature")
	git(t, dir, "branch", "other")

	got, err := Branches(context.Background(), dir, BranchOptions{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"main", "feature", "other"}, branchNames(got))
}

func TestBranchesMarksTheCurrentBranch(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	git(t, dir, "checkout", "-b", "feature")

	got, err := Branches(context.Background(), dir, BranchOptions{})
	require.NoError(t, err)
	require.True(t, branchNamed(t, got, "feature").Current)
	require.False(t, branchNamed(t, got, "main").Current)
}

func TestBranchesRecordsTheTipCommit(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "the subject line")

	got, err := Branches(context.Background(), dir, BranchOptions{})
	require.NoError(t, err)
	b := branchNamed(t, got, "main")
	require.Equal(t, "the subject line", b.LastSubject)
	require.Equal(t, "Test", b.LastAuthor)
	require.NotEmpty(t, b.LastCommit)
	require.False(t, b.LastDate.IsZero())
	require.Positive(t, b.Age())
}

// A merged branch is safe to delete; an unmerged one carries work that
// would be lost. Getting this backwards is the one dangerous mistake
// this listing can make.
func TestBranchesIdentifiesMergedBranches(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "init")

	git(t, dir, "checkout", "-b", "merged-work")
	writeFile(t, dir, "b.txt", "merged\n")
	commit(t, dir, "merged work")

	git(t, dir, "checkout", "main")
	git(t, dir, "merge", "--no-ff", "merged-work", "-m", "merge it")

	git(t, dir, "checkout", "-b", "unmerged-work")
	writeFile(t, dir, "c.txt", "unmerged\n")
	commit(t, dir, "unmerged work")
	git(t, dir, "checkout", "main")

	got, err := Branches(context.Background(), dir, BranchOptions{MergedBase: "main"})
	require.NoError(t, err)
	require.Equal(t, "main", branchNamed(t, got, "merged-work").MergedInto)
	require.Empty(t, branchNamed(t, got, "unmerged-work").MergedInto)
}

func TestBranchesRecordsDivergenceFromUpstream(t *testing.T) {
	origin := gitRepo(t)
	writeFile(t, origin, "a.txt", "one\n")
	commit(t, origin, "init")

	clone := t.TempDir()
	_, err := Run(context.Background(), clone, "clone", origin, ".")
	require.NoError(t, err)
	git(t, clone, "config", "user.email", "test@example.com")
	git(t, clone, "config", "user.name", "Test")

	writeFile(t, clone, "b.txt", "two\n")
	commit(t, clone, "local work")

	got, err := Branches(context.Background(), clone, BranchOptions{})
	require.NoError(t, err)
	b := branchNamed(t, got, "main")
	require.NotEmpty(t, b.Upstream)
	require.Equal(t, 1, b.Ahead)
	require.Zero(t, b.Behind)
}

func TestBranchesCanIncludeRemotes(t *testing.T) {
	origin := gitRepo(t)
	writeFile(t, origin, "a.txt", "one\n")
	commit(t, origin, "init")

	clone := t.TempDir()
	_, err := Run(context.Background(), clone, "clone", origin, ".")
	require.NoError(t, err)

	local, err := Branches(context.Background(), clone, BranchOptions{})
	require.NoError(t, err)
	for _, b := range local {
		require.False(t, b.Remote, "no remote refs without IncludeRemote")
	}

	withRemote, err := Branches(context.Background(), clone, BranchOptions{IncludeRemote: true})
	require.NoError(t, err)
	require.Greater(t, len(withRemote), len(local))

	var sawRemote bool
	for _, b := range withRemote {
		if b.Remote {
			sawRemote = true
		}
	}
	require.True(t, sawRemote)
}

// A branch named "feat/x" contains a slash but is local, and mislabelling
// it as a remote would hide it from a local-branch listing.
func TestBranchesDoNotMistakeASlashedNameForARemote(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	git(t, dir, "branch", "feat/some-topic")

	got, err := Branches(context.Background(), dir, BranchOptions{})
	require.NoError(t, err)
	require.False(t, branchNamed(t, got, "feat/some-topic").Remote)
}

func TestParseTrackReadsBothDirections(t *testing.T) {
	ahead, behind := parseTrack("[ahead 3, behind 1]")
	require.Equal(t, 3, ahead)
	require.Equal(t, 1, behind)

	ahead, behind = parseTrack("[ahead 2]")
	require.Equal(t, 2, ahead)
	require.Zero(t, behind)

	ahead, behind = parseTrack("[behind 5]")
	require.Zero(t, ahead)
	require.Equal(t, 5, behind)

	ahead, behind = parseTrack("")
	require.Zero(t, ahead)
	require.Zero(t, behind)

	// Malformed input must not panic or invent numbers.
	ahead, behind = parseTrack("[gone]")
	require.Zero(t, ahead)
	require.Zero(t, behind)
}

func TestCurrentBranchReturnsTheCheckedOutName(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	name, err := CurrentBranch(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "main", name)
}

// A detached HEAD has no branch name, and returning the literal "HEAD"
// would be useless.
func TestCurrentBranchReturnsTheCommitWhenDetached(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "two\n")
	commit(t, dir, "second")
	git(t, dir, "checkout", "--detach", "HEAD~1")

	name, err := CurrentBranch(context.Background(), dir)
	require.NoError(t, err)
	require.NotEqual(t, "HEAD", name)
	require.NotEmpty(t, name)
}

func TestDefaultBranchFindsAConventionalName(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	require.Equal(t, "main", DefaultBranch(context.Background(), dir))
}

// A wrong guess would mark unmerged work as merged, so an unrecognisable
// repository must return nothing rather than a plausible name.
func TestDefaultBranchReturnsNothingWhenItCannotTell(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	git(t, dir, "branch", "-m", "main", "some-unconventional-name")

	require.Empty(t, DefaultBranch(context.Background(), dir))
}

func TestBranchAgeIsZeroWithoutADate(t *testing.T) {
	require.Zero(t, Branch{}.Age())
	require.Positive(t, Branch{LastDate: time.Now().Add(-time.Hour)}.Age())
}

func TestBranchesReportsANonRepository(t *testing.T) {
	_, err := Branches(context.Background(), t.TempDir(), BranchOptions{})
	require.ErrorIs(t, err, ErrNotARepository)
}
