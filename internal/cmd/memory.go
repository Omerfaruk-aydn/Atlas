package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Inspect and clear the agent's persistent memory",
	Long:  "Inspect and clear the project and user memory the agent writes with its memory tool. See the memory tool's own description for what these stores are.",
}

var memoryShowCmd = &cobra.Command{
	Use:   "show [project|user]",
	Short: "Print a memory store's contents",
	Long:  "Print a memory store's contents. Omit the scope to print both.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMemoryShow,
}

var memoryClearCmd = &cobra.Command{
	Use:   "clear <project|user>",
	Short: "Empty a memory store",
	Args:  cobra.ExactArgs(1),
	RunE:  runMemoryClear,
}

func init() {
	memoryCmd.AddCommand(memoryShowCmd)
	memoryCmd.AddCommand(memoryClearCmd)
	rootCmd.AddCommand(memoryCmd)
}

// buildMemoryStore mirrors internal/agent's memoryStore. Reimplemented here
// rather than imported for the same reason as skill.go's
// discoverConfiguredSkills: internal/agent pulls in far more than a memory
// read/clear command needs.
func buildMemoryStore(cfg *config.ConfigStore) *memory.Store {
	opts := memory.Options{
		ProjectDir: filepath.Join(cfg.Config().Options.DataDirectory, "memory"),
		UserDir:    filepath.Join(home.Config(), "atlas"),
	}
	if m := cfg.Config().Options.Memory; m != nil {
		opts.ProjectLimit = m.ProjectLimit
		opts.UserLimit = m.UserLimit
	}
	return memory.New(opts)
}

func parseMemoryScope(arg string) (memory.Scope, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "project":
		return memory.ScopeProject, nil
	case "user":
		return memory.ScopeUser, nil
	default:
		return "", fmt.Errorf("unknown scope %q: use project or user", arg)
	}
}

func runMemoryShow(cmd *cobra.Command, args []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	store := buildMemoryStore(cfg)

	scopes := memory.Scopes
	if len(args) == 1 {
		scope, err := parseMemoryScope(args[0])
		if err != nil {
			return err
		}
		scopes = []memory.Scope{scope}
	}

	out := cmd.OutOrStdout()
	for _, scope := range scopes {
		content, err := store.Read(scope)
		if err != nil {
			return fmt.Errorf("reading %s memory: %w", scope, err)
		}
		fmt.Fprintf(out, "# %s (%s)\n", scope, home.Short(store.Path(scope)))
		if strings.TrimSpace(content) == "" {
			fmt.Fprintln(out, "(empty)")
		} else {
			fmt.Fprint(out, content)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func runMemoryClear(cmd *cobra.Command, args []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init(cwd, dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	scope, err := parseMemoryScope(args[0])
	if err != nil {
		return err
	}

	store := buildMemoryStore(cfg)
	if _, err := store.Set(scope, ""); err != nil {
		return fmt.Errorf("clearing %s memory: %w", scope, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Cleared %s memory (%s).\n", scope, home.Short(store.Path(scope)))
	return nil
}
