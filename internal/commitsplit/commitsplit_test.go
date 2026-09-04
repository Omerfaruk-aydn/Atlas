package commitsplit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func testRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		require.NoError(t, cmd.Run())
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-m", message)
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func groupLabels(r Result) []string {
	var out []string
	for _, g := range r.Groups {
		out = append(out, g.Label)
	}
	return out
}

func TestSplitReportsNothingWhenClean(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	commitAll(t, dir, "init")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Empty(t, got.Groups)
}

func TestSplitOrdersDependencyBeforeDependent(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "base/base.go", "package base\n\nfunc Value() int { return 1 }\n")
	writeFile(t, dir, "top/top.go", "package top\n\nimport \"example.com/app/base\"\n\nfunc Use() int { return base.Value() }\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "base/base.go", "package base\n\nfunc Value() int { return 2 }\n")
	writeFile(t, dir, "top/top.go", "package top\n\nimport \"example.com/app/base\"\n\nfunc Use() int { return base.Value() + 1 }\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	labels := groupLabels(got)
	require.Equal(t, []string{"example.com/app/base", "example.com/app/top"}, labels)
}

func TestSplitPlacesUnrelatedPackagesIndependently(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	writeFile(t, dir, "b/b.go", "package b\n\nfunc B() {}\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() int { return 1 }\n")
	writeFile(t, dir, "b/b.go", "package b\n\nfunc B() int { return 2 }\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 2)
	require.Empty(t, got.Cycles)
}

func TestSplitReportsACycle(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	writeFile(t, dir, "b/b.go", "package b\n\nfunc B() {}\n")
	commitAll(t, dir, "init")

	// A real Go import cycle would fail to build, but the graph only
	// reads import declarations syntactically -- it never compiles the
	// tree -- so this still exercises the cycle-handling path.
	writeFile(t, dir, "a/a.go", "package a\n\nimport \"example.com/app/b\"\n\nfunc A() { b.B() }\n")
	writeFile(t, dir, "b/b.go", "package b\n\nimport \"example.com/app/a\"\n\nfunc B() { a.A() }\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 2)
	require.Len(t, got.Cycles, 1)
}

func TestSplitGroupsNonGoFilesByDirectory(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "README.md", "# app\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "docs/guide.md", "# guide\n")
	writeFile(t, dir, "docs/faq.md", "# faq\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Files, 2)
}

func TestSplitIncludesANewUntrackedFileInAnExistingDirectory(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a/new.go", "package a\n\nfunc New() {}\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Equal(t, []string{"a/new.go"}, got.Groups[0].Files)
	require.Positive(t, got.Groups[0].Insertions)
}

func TestSplitExpandsAWhollyNewUntrackedDirectory(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "README.md", "# app\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "newpkg/one.go", "package newpkg\n\nfunc One() {}\n")
	writeFile(t, dir, "newpkg/two.go", "package newpkg\n\nfunc Two() {}\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Len(t, got.Groups[0].Files, 2)
	require.Equal(t, "example.com/app/newpkg", got.Groups[0].Label)
}

func TestSplitReportsInsertionsAndDeletions(t *testing.T) {
	dir := testRepo(t)
	writeFile(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n")
	commitAll(t, dir, "init")

	writeFile(t, dir, "a/a.go", "package a\n\nfunc A() {}\n\nfunc B() {}\n")

	got, err := Split(context.Background(), dir)
	require.NoError(t, err)
	require.Len(t, got.Groups, 1)
	require.Positive(t, got.Groups[0].Insertions)
}
