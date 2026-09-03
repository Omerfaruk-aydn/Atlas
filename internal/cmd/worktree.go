package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// worktreesSubdir is where `worktree new` creates worktrees, relative to
// the repository root -- a convention, not a git requirement, so several
// worktrees for one repo stay easy to find and to clean up together.
const worktreesSubdir = ".atlas/worktrees"

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees for isolated work",
	Long: "Create, list, and remove git worktrees under .atlas/worktrees, so a risky change -- or a " +
		"subagent's own workspace -- can be tried without touching the main working tree. A thin " +
		"wrapper around `git worktree`; the current directory must already be inside a git repository.",
}

var (
	worktreeNewBranch string
	worktreeNewBase   string
)

var worktreeNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new worktree on a new branch",
	Long: "Create a git worktree at .atlas/worktrees/<name>, on a new branch (named <name> by default, " +
		"or --branch). --base picks what the branch starts from; omit it to start from HEAD. " +
		"Fails if the path or branch already exists.",
	Args: cobra.ExactArgs(1),
	RunE: runWorktreeNew,
}

var worktreeListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List git worktrees",
	Long:    "List every worktree git knows about for this repository, not only ones `worktree new` created.",
	Args:    cobra.NoArgs,
	RunE:    runWorktreeList,
}

var worktreeRemoveForce bool

var worktreeRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove a worktree created by `worktree new`",
	Long: "Remove the worktree at .atlas/worktrees/<name>. Refuses one with uncommitted changes unless " +
		"--force is given. The branch itself is left alone -- deleting branches is a separate, more " +
		"consequential decision than removing a worktree.",
	Args: cobra.ExactArgs(1),
	RunE: runWorktreeRemove,
}

func init() {
	worktreeNewCmd.Flags().StringVar(&worktreeNewBranch, "branch", "", "branch to create (default: the worktree name)")
	worktreeNewCmd.Flags().StringVar(&worktreeNewBase, "base", "", "ref the new branch starts from (default: HEAD)")
	worktreeRemoveCmd.Flags().BoolVar(&worktreeRemoveForce, "force", false, "remove even with uncommitted changes")
	worktreeCmd.AddCommand(worktreeNewCmd)
	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeRemoveCmd)
	rootCmd.AddCommand(worktreeCmd)
}

// runGit runs git in dir and returns its combined output, trimmed. Errors
// carry that output, since git's most useful explanation of a failure is
// almost always on stdout/stderr, not in the exec error itself.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), trimmed)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return trimmed, nil
}

// repoRoot resolves the git repository root containing cwd.
func repoRoot(ctx context.Context, cwd string) (string, error) {
	out, err := runGit(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return filepath.FromSlash(out), nil
}

// worktreePath is where `worktree new`/`remove` put or expect to find a
// worktree named name, given the repository root.
func worktreePath(root, name string) string {
	return filepath.Join(root, worktreesSubdir, name)
}

func runWorktreeNew(cmd *cobra.Command, args []string) error {
	name := args[0]
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	root, err := repoRoot(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	path := worktreePath(root, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !os.IsNotExist(err) {
		return err
	}

	branch := worktreeNewBranch
	if branch == "" {
		branch = name
	}

	gitArgs := []string{"worktree", "add", "-b", branch, path}
	if worktreeNewBase != "" {
		gitArgs = append(gitArgs, worktreeNewBase)
	}
	if _, err := runGit(cmd.Context(), root, gitArgs...); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created worktree at %s on branch %s\n", path, branch)
	return nil
}

func runWorktreeList(cmd *cobra.Command, _ []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	root, err := repoRoot(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	out, err := runGit(cmd.Context(), root, "worktree", "list")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), out)
	return nil
}

func runWorktreeRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	root, err := repoRoot(cmd.Context(), cwd)
	if err != nil {
		return err
	}

	path := worktreePath(root, name)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("no worktree at %s: %w", path, err)
	}

	gitArgs := []string{"worktree", "remove"}
	if worktreeRemoveForce {
		gitArgs = append(gitArgs, "--force")
	}
	gitArgs = append(gitArgs, path)
	if _, err := runGit(cmd.Context(), root, gitArgs...); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed worktree at %s\n", path)
	return nil
}
