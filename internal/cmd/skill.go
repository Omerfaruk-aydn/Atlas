package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
)

var skillListJSON bool

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Inspect Agent Skills",
}

var skillListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List discovered skills",
	Long: "List every skill this workspace discovers -- builtin, project, and user -- and whether each is " +
		"enabled. Use --json for machine-readable output.",
	Args: cobra.NoArgs,
	RunE: runSkillList,
}

var skillShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a skill's full content",
	Args:  cobra.ExactArgs(1),
	RunE:  runSkillShow,
}

func init() {
	skillListCmd.Flags().BoolVar(&skillListJSON, "json", false, "output in JSON format")
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

	active := make(map[string]bool, len(activeSkills))
	for _, s := range activeSkills {
		active[s.Name] = true
	}

	sorted := append([]*skills.Skill(nil), allSkills...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	out := cmd.OutOrStdout()

	if skillListJSON {
		listed := make([]jsonSkill, 0, len(sorted))
		for _, s := range sorted {
			listed = append(listed, jsonSkill{
				Name:        s.Name,
				Origin:      skillOrigin(s, cwd),
				Enabled:     active[s.Name],
				Description: s.Description,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(listed)
	}

	if len(allSkills) == 0 {
		fmt.Fprintln(out, "No skills found.")
		return nil
	}

	for _, s := range sorted {
		status := "enabled"
		if !active[s.Name] {
			status = "disabled"
		}
		fmt.Fprintf(out, "%s (%s, %s)\n  %s\n", s.Name, skillOrigin(s, cwd), status, s.Description)
	}
	return nil
}

// jsonSkill is one skill's wire form for --json.
type jsonSkill struct {
	Name        string `json:"name"`
	Origin      string `json:"origin"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description"`
}

// skillOrigin reports where a skill came from: built into the binary, this
// project's .atlas/skills, or the user's own skills directory.
func skillOrigin(s *skills.Skill, cwd string) string {
	switch {
	case s.Builtin:
		return "builtin"
	case strings.HasPrefix(s.Path, cwd):
		return "project"
	default:
		return "user"
	}
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
