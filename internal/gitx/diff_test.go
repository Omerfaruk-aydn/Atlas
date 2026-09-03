package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func diffPaths(d Diff) []string {
	out := make([]string, 0, len(d.Files))
	for _, f := range d.Files {
		out = append(out, f.Path)
	}
	return out
}

func TestGetDiffSeesWorkingTreeChanges(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "one\ntwo\n")

	got, err := GetDiff(context.Background(), dir, DiffOptions{})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt"}, diffPaths(got))
	require.Equal(t, 1, got.Insertions)
	require.Zero(t, got.Deletions)
}

// Staged and unstaged are different questions, and conflating them is how
// a commit ends up containing the wrong thing.
func TestGetDiffSeparatesStagedFromUnstaged(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	writeFile(t, dir, "a.txt", "one\nstaged\n")
	git(t, dir, "add", "a.txt")
	writeFile(t, dir, "a.txt", "one\nstaged\nunstaged\n")

	staged, err := GetDiff(context.Background(), dir, DiffOptions{Staged: true})
	require.NoError(t, err)
	require.Equal(t, 1, staged.Insertions)

	unstaged, err := GetDiff(context.Background(), dir, DiffOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, unstaged.Insertions)
}

func TestGetDiffComparesAgainstARef(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "one\ntwo\n")
	commit(t, dir, "second")

	got, err := GetDiff(context.Background(), dir, DiffOptions{Ref: "HEAD~1"})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt"}, diffPaths(got))
	require.Equal(t, 1, got.Insertions)
}

func TestGetDiffComparesARange(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "init")
	git(t, dir, "checkout", "-b", "feature")
	writeFile(t, dir, "b.txt", "feature\n")
	commit(t, dir, "feature work")

	got, err := GetDiff(context.Background(), dir, DiffOptions{Ref: "main..feature"})
	require.NoError(t, err)
	require.Equal(t, []string{"b.txt"}, diffPaths(got))
}

func TestGetDiffNarrowsToAPath(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	writeFile(t, dir, "b.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "changed\n")
	writeFile(t, dir, "b.txt", "changed\n")

	got, err := GetDiff(context.Background(), dir, DiffOptions{Path: "a.txt"})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt"}, diffPaths(got))
}

func TestGetDiffOmitsPatchTextByDefault(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "changed\n")

	got, err := GetDiff(context.Background(), dir, DiffOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Files[0].Patch)
}

func TestGetDiffIncludesPatchWhenAsked(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "changed\n")

	got, err := GetDiff(context.Background(), dir, DiffOptions{WithPatch: true})
	require.NoError(t, err)
	require.Contains(t, got.Files[0].Patch, "-one")
	require.Contains(t, got.Files[0].Patch, "+changed")
	require.Contains(t, got.Files[0].Patch, "@@")
}

func TestGetDiffAttributesPatchesToTheRightFiles(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "a original\n")
	writeFile(t, dir, "b.txt", "b original\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "a changed\n")
	writeFile(t, dir, "b.txt", "b changed\n")

	got, err := GetDiff(context.Background(), dir, DiffOptions{WithPatch: true})
	require.NoError(t, err)
	require.Len(t, got.Files, 2)
	for _, f := range got.Files {
		switch f.Path {
		case "a.txt":
			require.Contains(t, f.Patch, "+a changed")
			require.NotContains(t, f.Patch, "b changed")
		case "b.txt":
			require.Contains(t, f.Patch, "+b changed")
			require.NotContains(t, f.Patch, "a changed")
		}
	}
}

func TestGetDiffRespectsThePatchByteBudget(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", strings.Repeat("a long replacement line\n", 200))

	got, err := GetDiff(context.Background(), dir, DiffOptions{WithPatch: true, MaxPatchBytes: 50})
	require.NoError(t, err)
	require.True(t, got.Truncated)
	require.Empty(t, got.Files[0].Patch)
	// The summary survives the truncation: that is the point of fetching
	// counts separately from text.
	require.Positive(t, got.Insertions)
}

// A rename is reported in a compact notation that names a path which does
// not exist. Expanding it is what keeps every later read from failing.
func TestGetDiffExpandsARenameIntoRealPaths(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "old.txt", "content long enough to be detected as a rename\n")
	commit(t, dir, "init")
	git(t, dir, "mv", "old.txt", "new.txt")

	got, err := GetDiff(context.Background(), dir, DiffOptions{Staged: true})
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
	require.Equal(t, "new.txt", got.Files[0].Path)
	require.Equal(t, "old.txt", got.Files[0].OrigPath)
}

func TestGetDiffExpandsABracedRename(t *testing.T) {
	orig, path, ok := splitRename("dir/{old => new}/file.txt")
	require.True(t, ok)
	require.Equal(t, "dir/old/file.txt", orig)
	require.Equal(t, "dir/new/file.txt", path)
}

// "a/{ => sub}/f" leaves a doubled slash if the empty segment is not
// cleaned up, producing a path that does not resolve.
func TestSplitRenameCleansUpAnEmptySegment(t *testing.T) {
	orig, path, ok := splitRename("a/{ => sub}/f.txt")
	require.True(t, ok)
	require.Equal(t, "a/f.txt", orig)
	require.Equal(t, "a/sub/f.txt", path)
}

func TestSplitRenameIgnoresANonRename(t *testing.T) {
	_, _, ok := splitRename("just/a/path.txt")
	require.False(t, ok)
}

func TestGetDiffMarksBinaryFiles(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	commit(t, dir, "init")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "blob.bin"),
		[]byte{0x00, 0x01, 0x02, 0xff, 0x00}, 0o644))
	git(t, dir, "add", "-A")

	got, err := GetDiff(context.Background(), dir, DiffOptions{Staged: true})
	require.NoError(t, err)
	require.Len(t, got.Files, 1)
	require.True(t, got.Files[0].Binary)
}

func TestGetDiffOnACleanTreeReturnsNothing(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	got, err := GetDiff(context.Background(), dir, DiffOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Files)
	require.Zero(t, got.Insertions)
}

func TestGetDiffHonoursContextLines(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "1\n2\n3\n4\nCHANGED\n6\n7\n8\n9\n")

	tight, err := GetDiff(context.Background(), dir, DiffOptions{WithPatch: true, ContextLines: 1})
	require.NoError(t, err)
	wide, err := GetDiff(context.Background(), dir, DiffOptions{WithPatch: true, ContextLines: 5})
	require.NoError(t, err)

	require.Less(t, len(tight.Files[0].Patch), len(wide.Files[0].Patch))
}

func TestGetDiffReportsANonRepository(t *testing.T) {
	_, err := GetDiff(context.Background(), t.TempDir(), DiffOptions{})
	require.ErrorIs(t, err, ErrNotARepository)
}
