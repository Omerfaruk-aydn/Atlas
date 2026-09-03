package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
)

var modelRolesJSON bool

var modelRolesCmd = &cobra.Command{
	Use:   "roles",
	Short: "List named model roles a subagent's model field can reference",
	Long: "List the model roles this workspace has configured -- \"large\" and \"small\" (the built-in " +
		"model types) plus any custom names in Options.ModelRoles -- and the provider/model each resolves " +
		"to. Use --json for machine-readable output.",
	Args: cobra.NoArgs,
	RunE: runModelRoles,
}

func init() {
	modelRolesCmd.Flags().BoolVar(&modelRolesJSON, "json", false, "output in JSON format")
	modelsCmd.AddCommand(modelRolesCmd)
}

// jsonModelRole is one role's wire form for --json.
type jsonModelRole struct {
	Role     string `json:"role"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

func runModelRoles(cmd *cobra.Command, _ []string) error {
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
	return listModelRoles(cmd, cfg)
}

// listModelRoles is split out from runModelRoles for the same reason
// listProviders is split from runProviderList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func listModelRoles(cmd *cobra.Command, cfg *config.ConfigStore) error {
	names := []string{"large", "small"}
	if opts := cfg.Config().Options; opts != nil {
		for name := range opts.ModelRoles {
			names = append(names, name)
		}
	}
	sort.Strings(names[2:]) // keep large/small first, custom roles sorted after

	out := cmd.OutOrStdout()

	if modelRolesJSON {
		roles := make([]jsonModelRole, 0, len(names))
		for _, name := range names {
			model, ok := cfg.Config().ResolveRole(name)
			if !ok {
				continue
			}
			roles = append(roles, jsonModelRole{Role: name, Provider: model.Provider, Model: model.Model})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(roles)
	}

	var shown int
	for _, name := range names {
		model, ok := cfg.Config().ResolveRole(name)
		if !ok {
			continue
		}
		fmt.Fprintf(out, "%s: %s/%s\n", name, model.Provider, model.Model)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(out, "No model roles configured.")
	}
	return nil
}
