package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/spf13/cobra"
)

var (
	agentNewDescription string
	agentNewModel       string
	agentNewUser        bool
)

var agentNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Scaffold a new subagent",
	Long: "Write a new subagent definition, ready to edit. Creates it in this project (.atlas/agents) " +
		"unless --user is given, in which case it goes in the user subagents directory. " +
		"Refuses to overwrite an existing subagent.",
	Args: cobra.ExactArgs(1),
	RunE: runAgentNew,
}

func init() {
	agentNewCmd.Flags().StringVarP(&agentNewDescription, "description", "d", "", "when this subagent should be used")
	agentNewCmd.Flags().StringVarP(&agentNewModel, "model", "m", "",
		"model role this subagent runs on (see `atlas models roles`); empty runs on the session's primary model")
	agentNewCmd.Flags().BoolVar(&agentNewUser, "user", false, "create it in the user subagents directory instead of this project")
	agentCmd.AddCommand(agentNewCmd)
}

const defaultSubagentDescription = "TODO: say when this subagent should be used."

const subagentTemplate = `# %s

TODO: the instructions this subagent runs with.
`

func runAgentNew(cmd *cobra.Command, args []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	// No config.Init here: this writes a file into one of the two
	// directories discovery already scans, and needs nothing from the
	// merged configuration to do it -- mirrors skill_new.go.
	dir := filepath.Join(cwd, ".atlas", "agents")
	if agentNewUser {
		dir = filepath.Join(home.Config(), "atlas", "agents")
	}
	return createSubagent(cmd, dir, args[0])
}

// createSubagent is split out from runAgentNew so a test can drive it
// against a directory of its own, mirroring createSkill.
func createSubagent(cmd *cobra.Command, dir, name string) error {
	path := subagents.Path(dir, name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; edit it, or pick another name", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	description := agentNewDescription
	if description == "" {
		description = defaultSubagentDescription
	}

	written, err := subagents.Save(dir, &subagents.Subagent{
		Name:         name,
		Description:  description,
		Model:        agentNewModel,
		Instructions: fmt.Sprintf(subagentTemplate, name),
	})
	if err != nil {
		return fmt.Errorf("creating the subagent: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", home.Short(written))
	if agentNewDescription == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "Set its description before relying on it.")
	}
	return nil
}
