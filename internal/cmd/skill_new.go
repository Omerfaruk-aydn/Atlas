package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/spf13/cobra"
)

var (
	skillNewDescription   string
	skillNewUser          bool
	skillNewUserInvocable bool
)

var skillNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a new skill",
	Long: "Write a new SKILL.md with valid frontmatter, ready to edit. Creates it in this project " +
		"(.atlas/skills) unless --user is given, in which case it goes in the user skills directory. " +
		"Refuses to overwrite an existing skill.",
	Args: cobra.ExactArgs(1),
	RunE: runSkillNew,
}

func init() {
	skillNewCmd.Flags().StringVarP(&skillNewDescription, "description", "d", "", "when the model should reach for this skill")
	skillNewCmd.Flags().BoolVar(&skillNewUser, "user", false, "create it in the user skills directory instead of this project")
	skillNewCmd.Flags().BoolVar(&skillNewUserInvocable, "user-invocable", false, "let the user invoke it by name as a slash command")
	skillCmd.AddCommand(skillNewCmd)
}

// defaultSkillDescription is used when none is given. It is deliberately a
// sentence the author has to replace: a skill is selected by its
// description, so an empty or generic one makes it unreachable, and
// frontmatter requires the field to be present at all.
const defaultSkillDescription = "TODO: say when the model should use this skill."

// skillTemplate is the body a new skill starts with. It is a prompt for the
// author, not filler: the headings are the ones a skill that works tends to
// have.
const skillTemplate = `# %s

## When to use this

TODO: the situations this applies to.

## Steps

1. TODO
`

func runSkillNew(cmd *cobra.Command, args []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	// No config.Init here: this writes a file into one of the two
	// directories discovery already scans, and needs nothing from the
	// merged configuration to do it.
	dir := filepath.Join(cwd, ".atlas", "skills")
	if skillNewUser {
		dir = filepath.Join(home.Config(), "atlas", "skills")
	}
	return createSkill(cmd, dir, args[0])
}

// createSkill is split out from runSkillNew so a test can drive it against
// a directory of its own without going through config.Init.
func createSkill(cmd *cobra.Command, dir, name string) error {
	path := skills.SkillPath(dir, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; edit it, or pick another name", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	description := skillNewDescription
	if description == "" {
		description = defaultSkillDescription
	}

	written, err := skills.Save(dir, &skills.Skill{
		Name:          name,
		Description:   description,
		UserInvocable: skillNewUserInvocable,
		Instructions:  fmt.Sprintf(skillTemplate, name),
	})
	if err != nil {
		return fmt.Errorf("creating the skill: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", home.Short(written))
	if skillNewDescription == "" {
		fmt.Fprintln(cmd.OutOrStdout(),
			"Set its description before relying on it: the model picks a skill by that line alone.")
	}
	return nil
}
