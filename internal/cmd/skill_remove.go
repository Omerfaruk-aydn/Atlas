package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
)

var skillRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a project or user skill",
	Long: "Delete the directory a project or user skill lives in. Refuses to touch a builtin skill -- " +
		"disable it in config instead -- and refuses a name that resolves outside a skills directory.",
	Args: cobra.ExactArgs(1),
	RunE: runSkillRemove,
}

func init() {
	skillCmd.AddCommand(skillRemoveCmd)
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
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
	return removeSkill(cmd, allSkills, args[0])
}

// removeSkill is split out from runSkillRemove so a test can drive it
// against a discovered skill list of its own, the same way createSkill is
// split from runSkillNew.
func removeSkill(cmd *cobra.Command, allSkills []*skills.Skill, name string) error {
	skill, ok := skills.Find(allSkills, name)
	if !ok {
		return fmt.Errorf("no skill named %q found", name)
	}
	if skill.Builtin {
		return fmt.Errorf("%q is a builtin skill and cannot be removed; disable it in config instead", name)
	}

	dir := skill.Path
	// The skill's own directory must be named after the skill: this is
	// what SkillPath produces on write, and refusing anything else stops a
	// crafted or unusual discovery path from making this remove a
	// directory the caller did not expect.
	if filepath.Base(dir) != skill.Name {
		return fmt.Errorf("refusing to remove %s: its directory does not match the skill name", home.Short(dir))
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing %s: %w", home.Short(dir), err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed %s\n", home.Short(dir))
	return nil
}
