package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/spf13/cobra"
)

var providerUsageJSON bool

var providerUsageCmd = &cobra.Command{
	Use:   "usage [name]",
	Short: "Show API key rotation state for configured providers",
	Long: "Show, for each provider with more than one API key configured (ProviderConfig.APIKeys), how " +
		"many keys it has and which one the next session or model rebuild would pick -- the persisted " +
		"round-robin state a live session's credential rotator reads and writes. Does not contact any " +
		"provider. [name] limits the output to one provider. Use --json for machine-readable output.",
	Args: cobra.MaximumNArgs(1),
	RunE: runProviderUsage,
}

func init() {
	providerUsageCmd.Flags().BoolVar(&providerUsageJSON, "json", false, "output in JSON format")
	providerCmd.AddCommand(providerUsageCmd)
}

func runProviderUsage(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init("", dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	var name string
	if len(args) == 1 {
		name = args[0]
	}
	return listProviderUsage(cmd, cfg, name)
}

// jsonProviderUsage is one provider's wire form for --json. The actual key
// value is deliberately never included: a key template (e.g. "$OPENAI_KEY")
// is not a secret itself, but the point of this command is to show rotation
// state, not to become another place a provider's key list gets echoed.
type jsonProviderUsage struct {
	Name       string `json:"name"`
	Keys       int    `json:"keys"`
	NextKeyIdx int    `json:"next_key_index"`
	NextKeyOf  int    `json:"next_key_of"`
}

// listProviderUsage is split out from runProviderUsage for the same reason
// listProviders is split from runProviderList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func listProviderUsage(cmd *cobra.Command, cfg *config.ConfigStore, filterName string) error {
	state := credentials.ReadState(filepath.Join(cfg.Config().Options.DataDirectory, credentials.StateFileName))

	var names []string
	for providerID := range cfg.Config().Providers.Seq2() {
		names = append(names, providerID)
	}
	sort.Strings(names)

	out := cmd.OutOrStdout()
	var usages []jsonProviderUsage
	for _, name := range names {
		if filterName != "" && name != filterName {
			continue
		}
		providerCfg, _ := cfg.Config().Providers.Get(name)
		keys := credentials.CandidateAPIKeys(providerCfg.APIKey, providerCfg.APIKeys)
		if len(keys) < 2 {
			continue
		}
		nextIdx := state.Next[name] % len(keys)
		usages = append(usages, jsonProviderUsage{Name: name, Keys: len(keys), NextKeyIdx: nextIdx, NextKeyOf: len(keys)})
	}

	if providerUsageJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(usages)
	}

	if len(usages) == 0 {
		if filterName != "" {
			fmt.Fprintf(out, "%s has no rotation configured (0 or 1 API keys).\n", filterName)
		} else {
			fmt.Fprintln(out, "No provider has more than one API key configured; nothing to rotate.")
		}
		return nil
	}

	for _, u := range usages {
		fmt.Fprintf(out, "%s: %d keys, next pick is key #%d of %d\n", u.Name, u.Keys, u.NextKeyIdx+1, u.NextKeyOf)
	}
	return nil
}
