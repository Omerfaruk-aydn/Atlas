package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a git repository with one commit, so `worktree add`
// (which needs a valid ref to branch from) has something to work with.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.CommandContext(t.Context(), "git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0o644))
	run("add", "README.md")
	run("commit", "-q", "-m", "initial")
	return dir
}

func newWorktreeTestCmd(t *testing.T, runE func(*cobra.Command, []string) error, cwd string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runE}
	c.Flags().String("cwd", cwd, "")
	c.SetContext(t.Context())
	orig, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return c
}

func TestWorktreeNewCreatesAWorktreeAndBranch(t *testing.T) {
	repo := initTestRepo(t)
	c := newWorktreeTestCmd(t, runWorktreeNew, repo)
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"feature-x"}))
	require.Contains(t, out.String(), "Created worktree")

	path := filepath.Join(repo, ".atlas", "worktrees", "feature-x")
	require.DirExists(t, path)
	require.FileExists(t, filepath.Join(path, "README.md"))
}

func TestWorktreeNewRefusesADuplicateName(t *testing.T) {
	repo := initTestRepo(t)
	c := newWorktreeTestCmd(t, runWorktreeNew, repo)
	require.NoError(t, c.RunE(c, []string{"dup"}))

	c2 := newWorktreeTestCmd(t, runWorktreeNew, repo)
	err := c2.RunE(c2, []string{"dup"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestWorktreeNewWithCustomBranch(t *testing.T) {
	repo := initTestRepo(t)
	c := newWorktreeTestCmd(t, runWorktreeNew, repo)
	c.Flags().StringVar(&worktreeNewBranch, "branch", "custom-branch", "")
	t.Cleanup(func() { worktreeNewBranch = "" })
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, []string{"named"}))
	require.Contains(t, out.String(), "on branch custom-branch")
}

func TestWorktreeNewOutsideAGitRepoFails(t *testing.T) {
	c := newWorktreeTestCmd(t, runWorktreeNew, t.TempDir())
	err := c.RunE(c, []string{"x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a git repository")
}

func TestWorktreeListShowsTheMainAndCreatedWorktrees(t *testing.T) {
	repo := initTestRepo(t)
	newCmd := newWorktreeTestCmd(t, runWorktreeNew, repo)
	require.NoError(t, newCmd.RunE(newCmd, []string{"feature-y"}))

	listCmd := newWorktreeTestCmd(t, runWorktreeList, repo)
	var out bytes.Buffer
	listCmd.SetOut(&out)

	require.NoError(t, listCmd.RunE(listCmd, nil))
	got := out.String()
	// git may quote/escape the path (e.g. a non-ASCII username component),
	// so assert on the branch markers rather than the literal repo string.
	require.Contains(t, got, "[master]")
	require.Contains(t, got, "feature-y")
	require.Contains(t, got, "[feature-y]")
}

func TestWorktreeRemoveDeletesTheWorktree(t *testing.T) {
	repo := initTestRepo(t)
	newCmd := newWorktreeTestCmd(t, runWorktreeNew, repo)
	require.NoError(t, newCmd.RunE(newCmd, []string{"to-remove"}))

	path := filepath.Join(repo, ".atlas", "worktrees", "to-remove")
	require.DirExists(t, path)

	rmCmd := newWorktreeTestCmd(t, runWorktreeRemove, repo)
	var out bytes.Buffer
	rmCmd.SetOut(&out)
	require.NoError(t, rmCmd.RunE(rmCmd, []string{"to-remove"}))

	require.NoDirExists(t, path)
}

func TestWorktreeRemoveRejectsAnUnknownName(t *testing.T) {
	repo := initTestRepo(t)
	c := newWorktreeTestCmd(t, runWorktreeRemove, repo)
	err := c.RunE(c, []string{"nope"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no worktree at")
}
