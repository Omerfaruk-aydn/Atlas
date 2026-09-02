package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Inspect Agent Skills",
}

var skillListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List discovered skills",
	Long:    "List every skill this workspace discovers -- builtin, project, and user -- and whether each is enabled.",
	Args:    cobra.NoArgs,
	RunE:    runSkillList,
}

var skillShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a skill's full content",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillShow,
}

func init() {
	skillCmd.AddCommand(skillListCmd)
	skillCmd.AddCommand(skillShowCmd)
	rootCmd.AddCommand(skillCmd)
}

// discoverConfiguredSkills mirrors internal/agent's discoverSkills, kept
// separate rather than imported so this lightweight CLI command does not
// pull in the agent package's full dependency graph (providers, LSP, MCP)
// just to list files.
func discoverConfiguredSkills(cfg *config.ConfigStore) (allSkills, activeSkills []*skills.Skill) {
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
	allSkills, activeSkills, _ = skills.DiscoverFromConfig(skills.DiscoveryConfig{
		SkillsPaths:    paths,
		DisabledSkills: disabled,
		Resolver:       resolver,
	})
	return allSkills, activeSkills
}

func runSkillList(cmd *cobra.Command, _ []string) error {
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

	allSkills, activeSkills := discoverConfiguredSkills(cfg)
	if len(allSkills) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No skills found.")
		return nil
	}

	active := make(map[string]bool, len(activeSkills))
	for _, s := range activeSkills {
		active[s.Name] = true
	}

	sorted := append([]*skills.Skill(nil), allSkills...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	out := cmd.OutOrStdout()
	for _, s := range sorted {
		origin := "user"
		switch {
		case s.Builtin:
			origin = "builtin"
		case strings.HasPrefix(s.Path, cwd):
			origin = "project"
		}
		status := "enabled"
		if !active[s.Name] {
			status = "disabled"
		}
		fmt.Fprintf(out, "%s (%s, %s)\n  %s\n", s.Name, origin, status, s.Description)
	}
	return nil
}

func runSkillShow(cmd *cobra.Command, args []string) error {
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

	allSkills, _ := discoverConfiguredSkills(cfg)
	skill, ok := skills.Find(allSkills, args[0])
	if !ok {
		return fmt.Errorf("no skill named %q found", args[0])
	}

	rendered, err := skills.Render(skill)
	if err != nil {
		return fmt.Errorf("rendering skill: %w", err)
	}
	_, err = cmd.OutOrStdout().Write(rendered)
	return err
}
