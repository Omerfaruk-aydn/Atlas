package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/spf13/cobra"
)

var agentRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a subagent definition",
	Long:  "Delete the file a discovered subagent's definition lives in.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAgentRemove,
}

func init() {
	agentCmd.AddCommand(agentRemoveCmd)
}

func runAgentRemove(cmd *cobra.Command, args []string) error {
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

	all := discoverConfiguredSubagents(cfg)
	return removeSubagent(cmd, all, args[0])
}

// removeSubagent is split out from runAgentRemove so a test can drive it
// against a discovered subagent list of its own, mirroring removeSkill.
func removeSubagent(cmd *cobra.Command, all []*subagents.Subagent, name string) error {
	s, ok := subagents.Find(all, name)
	if !ok {
		return fmt.Errorf("no subagent named %q found", name)
	}

	// s.Path is the file itself (unlike a skill's directory); the file's
	// own base name must match the subagent's name, mirroring the
	// directory-name check skill_remove.go makes, so a discovery quirk
	// never causes the wrong file to be deleted.
	base := filepath.Base(s.Path)
	if base != name+subagents.FileExt {
		return fmt.Errorf("refusing to remove %s: its file name does not match the subagent name", home.Short(s.Path))
	}

	if err := os.Remove(s.Path); err != nil {
		return fmt.Errorf("removing %s: %w", home.Short(s.Path), err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", home.Short(s.Path))
	return nil
}
