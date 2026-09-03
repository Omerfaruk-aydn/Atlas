package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// gitRepo makes a real repository in a temp directory. Parsing git's
// output is only meaningfully tested against git's actual output --
// hand-written fixtures test the fixture, not the parser.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	ctx := context.Background()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		_, err := Run(ctx, dir, args...)
		require.NoError(t, err)
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	_, err := Run(context.Background(), dir, args...)
	require.NoError(t, err)
}

func commit(t *testing.T, dir, message string) {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", message)
}

func fileByPath(t *testing.T, s Status, path string) FileStatus {
	t.Helper()
	for _, f := range s.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no status for %q (have %+v)", path, s.Files)
	return FileStatus{}
}

func TestGetStatusOnACleanRepo(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	commit(t, dir, "init")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.True(t, got.Clean())
	require.Equal(t, "main", got.Branch)
	require.False(t, got.Detached)
}

func TestGetStatusSeesAnUntrackedFile(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "hello\n")
	commit(t, dir, "init")
	writeFile(t, dir, "new.txt", "new\n")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "untracked", fileByPath(t, got, "new.txt").Unstaged)
}

// A file changed in the index and again in the working tree has to show
// on both sides, or a commit will silently capture only half of it.
func TestGetStatusSeparatesStagedFromUnstaged(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	writeFile(t, dir, "a.txt", "two\n")
	git(t, dir, "add", "a.txt")
	writeFile(t, dir, "a.txt", "three\n")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	f := fileByPath(t, got, "a.txt")
	require.Equal(t, "modified", f.Staged)
	require.Equal(t, "modified", f.Unstaged)
	require.Equal(t, 1, got.StagedCount())
	require.Equal(t, 1, got.UnstagedCount())
}

func TestGetStatusSeesAStagedAddition(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "b.txt", "two\n")
	git(t, dir, "add", "b.txt")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "added", fileByPath(t, got, "b.txt").Staged)
}

func TestGetStatusSeesADeletion(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	require.NoError(t, os.Remove(filepath.Join(dir, "a.txt")))

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "deleted", fileByPath(t, got, "a.txt").Unstaged)
}

// A rename record carries two paths inside one NUL-terminated record. A
// parser that treats every NUL as a record boundary loses sync here and
// misreads everything after it.
func TestGetStatusParsesARenameWithoutLosingSync(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "old.txt", "content that is long enough to be detected as a rename\n")
	writeFile(t, dir, "other.txt", "unrelated\n")
	commit(t, dir, "init")

	git(t, dir, "mv", "old.txt", "new.txt")
	writeFile(t, dir, "other.txt", "changed\n")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)

	renamed := fileByPath(t, got, "new.txt")
	require.Equal(t, "renamed", renamed.Staged)
	require.Equal(t, "old.txt", renamed.OrigPath)

	// The record after the rename must still parse correctly.
	require.Equal(t, "modified", fileByPath(t, got, "other.txt").Unstaged)
}

func TestGetStatusHandlesAPathWithSpaces(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a file with spaces.txt", "y\n")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.Equal(t, "untracked", fileByPath(t, got, "a file with spaces.txt").Unstaged)
}

func TestGetStatusReportsADetachedHead(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "two\n")
	commit(t, dir, "second")

	git(t, dir, "checkout", "--detach", "HEAD~1")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.True(t, got.Detached)
	// The branch field must hold something identifying, not the literal
	// "(detached)".
	require.NotEqual(t, "(detached)", got.Branch)
	require.Len(t, got.Branch, 8)
}

func TestGetStatusReportsUnmergedPaths(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "base\n")
	commit(t, dir, "init")

	git(t, dir, "checkout", "-b", "other")
	writeFile(t, dir, "a.txt", "theirs\n")
	commit(t, dir, "theirs")

	git(t, dir, "checkout", "main")
	writeFile(t, dir, "a.txt", "ours\n")
	commit(t, dir, "ours")

	// The merge is expected to fail; that is the point.
	_, _ = Run(context.Background(), dir, "merge", "other")

	got, err := GetStatus(context.Background(), dir)
	require.NoError(t, err)
	require.Contains(t, got.Conflicts, "a.txt")
	require.Equal(t, "unmerged", fileByPath(t, got, "a.txt").Staged)
}

func TestGetStatusReportsDivergenceFromUpstream(t *testing.T) {
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

	got, err := GetStatus(context.Background(), clone)
	require.NoError(t, err)
	require.NotEmpty(t, got.Upstream)
	require.Equal(t, 1, got.Ahead)
	require.Equal(t, 0, got.Behind)
}

// "This is not a repository" is an answer the caller can act on, not a
// failure to report as an internal error.
func TestGetStatusReportsANonRepositoryDistinctly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	_, err := GetStatus(context.Background(), t.TempDir())
	require.ErrorIs(t, err, ErrNotARepository)
}

func TestIsRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	require.True(t, IsRepository(context.Background(), gitRepo(t)))
	require.False(t, IsRepository(context.Background(), t.TempDir()))
}

func TestRootReturnsTheTopLevel(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "sub/a.txt", "x\n")
	commit(t, dir, "init")

	root, err := Root(context.Background(), filepath.Join(dir, "sub"))
	require.NoError(t, err)
	// macOS hands out symlinked temp dirs, so compare resolved paths.
	wantResolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	gotResolved, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	require.Equal(t, wantResolved, gotResolved)
}

func TestParseAheadBehind(t *testing.T) {
	ahead, behind := parseAheadBehind("+3 -2")
	require.Equal(t, 3, ahead)
	require.Equal(t, 2, behind)

	ahead, behind = parseAheadBehind("+0 -0")
	require.Zero(t, ahead)
	require.Zero(t, behind)

	// Malformed input must not panic or produce nonsense.
	ahead, behind = parseAheadBehind("garbage")
	require.Zero(t, ahead)
	require.Zero(t, behind)
}

func TestExpandXY(t *testing.T) {
	staged, unstaged := expandXY("M.")
	require.Equal(t, "modified", staged)
	require.Empty(t, unstaged)

	staged, unstaged = expandXY(".D")
	require.Empty(t, staged)
	require.Equal(t, "deleted", unstaged)

	staged, unstaged = expandXY("bad-length")
	require.Empty(t, staged)
	require.Empty(t, unstaged)
}
