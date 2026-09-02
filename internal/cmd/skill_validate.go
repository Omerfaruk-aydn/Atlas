package cmd

import (
	"fmt"
	"io"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
)

var skillValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check that every discovered skill file parses",
	Long: "Check every skill file this workspace discovers and report the ones that failed to parse or " +
		"validate. A broken skill is skipped silently at startup, so this is the only way to find out why " +
		"one never shows up in `skill list`. Exits non-zero if any file failed.",
	Args: cobra.NoArgs,
	RunE: runSkillValidate,
}

func init() {
	skillCmd.AddCommand(skillValidateCmd)
}

func runSkillValidate(cmd *cobra.Command, _ []string) error {
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

	return printSkillStates(cmd.OutOrStdout(), discoverSkillStates(cfg))
}

// discoverSkillStates runs the same discovery `skill list` does but keeps
// the per-file states that listing discards -- those states are where a
// parse or validation failure is recorded.
func discoverSkillStates(cfg *config.ConfigStore) []*skills.SkillState {
	opts := cfg.Config().Options
	var paths, disabled []string
	if opts != nil {
		paths = opts.SkillsPaths
		disabled = opts.DisabledSkills
	}
	var resolver func(string) (string, error)
	if r := cfg.Resolver(); r != nil {
		resolver = r.ResolveValue
	}
	_, _, states := skills.DiscoverFromConfig(skills.DiscoveryConfig{
		SkillsPaths:    paths,
		DisabledSkills: disabled,
		Resolver:       resolver,
	})
	return states
}

func printSkillStates(out io.Writer, states []*skills.SkillState) error {
	if len(states) == 0 {
		_, err := fmt.Fprintln(out, "No skill files found.")
		return err
	}

	var failed int
	for _, s := range states {
		if s.State != skills.StateError {
			continue
		}
		failed++
		fmt.Fprintf(out, "%s: %s\n", home.Short(s.Path), s.Err)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d skill files failed to load", failed, len(states))
	}
	fmt.Fprintf(out, "All %d skill files loaded.\n", len(states))
	return nil
}
