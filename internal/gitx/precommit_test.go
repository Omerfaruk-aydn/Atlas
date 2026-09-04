package gitx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func findPreCommitKind(t *testing.T, findings []PreCommitFinding, kind string) *PreCommitFinding {
	t.Helper()
	for i := range findings {
		if findings[i].Kind == kind {
			return &findings[i]
		}
	}
	return nil
}

func TestPreCommitCheckFlagsAMergeConflictMarker(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.txt", "one\n<<<<<<< HEAD\ntwo\n=======\nthree\n>>>>>>> branch\n")
	git(t, dir, "add", "a.txt")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	f := findPreCommitKind(t, got.Findings, "merge-conflict-marker")
	require.NotNil(t, f)
	require.Equal(t, "a.txt", f.File)
}

func TestPreCommitCheckFlagsADebugStatement(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.go", "package a\n\nfunc f() {\n\tfmt.Println(\"debug\", x)\n}\n")
	git(t, dir, "add", "a.go")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.NotNil(t, findPreCommitKind(t, got.Findings, "debug-statement"))
}

func TestPreCommitCheckIgnoresARemovedDebugStatement(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.go", "package a\n\nfunc f() {\n\tconsole.log(x)\n}\n")
	commit(t, dir, "init")
	writeFile(t, dir, "a.go", "package a\n\nfunc f() {}\n")
	git(t, dir, "add", "a.go")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.Nil(t, findPreCommitKind(t, got.Findings, "debug-statement"))
}

func TestPreCommitCheckFlagsALargeFile(t *testing.T) {
	dir := gitRepo(t)
	big := strings.Repeat("x", 2048)
	writeFile(t, dir, "big.bin", big)
	git(t, dir, "add", "big.bin")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{MaxFileBytes: 1024})
	require.NoError(t, err)
	f := findPreCommitKind(t, got.Findings, "large-file")
	require.NotNil(t, f)
	require.Equal(t, "big.bin", f.File)
}

func TestPreCommitCheckAcceptsASmallFile(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "small.txt", "hello\n")
	git(t, dir, "add", "small.txt")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{MaxFileBytes: 1024})
	require.NoError(t, err)
	require.Nil(t, findPreCommitKind(t, got.Findings, "large-file"))
}

func TestPreCommitCheckReportsANonRepository(t *testing.T) {
	_, err := PreCommitCheck(context.Background(), t.TempDir(), PreCommitOptions{})
	require.ErrorIs(t, err, ErrNotARepository)
}

func TestPreCommitCheckReportsNothingWhenClean(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.Empty(t, got.Findings)
	require.Equal(t, 0, got.FilesStaged)
}

func TestPreCommitCheckCountsFilesStaged(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	writeFile(t, dir, "b.txt", "two\n")
	git(t, dir, "add", "-A")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.Equal(t, 2, got.FilesStaged)
}

func TestPreCommitCheckDefaultThreshold(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "small.txt", "hello\n")
	git(t, dir, "add", "small.txt")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.Nil(t, findPreCommitKind(t, got.Findings, "large-file"))
}

func TestPreCommitCheckHandlesADeletedFileWithoutCrashing(t *testing.T) {
	dir := gitRepo(t)
	writeFile(t, dir, "a.txt", "one\n")
	commit(t, dir, "init")
	require.NoError(t, os.Remove(filepath.Join(dir, "a.txt")))
	git(t, dir, "add", "-A")

	got, err := PreCommitCheck(context.Background(), dir, PreCommitOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, got.FilesStaged)
}
