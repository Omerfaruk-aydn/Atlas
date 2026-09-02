package cmd

import (
	"errors"
	"fmt"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Inspect configured providers",
}

var providerTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Check that configured providers' credentials work",
	Long: "Check that a configured provider's credentials work, without starting a session. " +
		"Makes one request to the provider (e.g. listing models) and reports whether it succeeded. " +
		"[name] is the provider ID as it appears in the config file's providers map, or in `atlas models`. " +
		"Omit it to check every configured, enabled provider.",
	Args: cobra.MaximumNArgs(1),
	RunE: runProviderTest,
}

var providerListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured providers",
	Long:    "List configured providers, their type, and whether an API key is set. Does not contact any provider -- see `provider test` for that.",
	Args:    cobra.NoArgs,
	RunE:    runProviderList,
}

func init() {
	providerCmd.AddCommand(providerTestCmd)
	providerCmd.AddCommand(providerListCmd)
	rootCmd.AddCommand(providerCmd)
}

func runProviderTest(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init("", dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	if len(args) == 1 {
		return testOneProvider(cmd, cfg, args[0])
	}
	return testAllProviders(cmd, cfg)
}

// testAllProviders checks every configured, enabled provider and reports
// each one's result, split out from runProviderTest so it can be driven
// against a *config.ConfigStore built directly in a test -- runProviderTest
// itself always builds its own via config.Init, which a test cannot inject
// a fake into.
func testAllProviders(cmd *cobra.Command, cfg *config.ConfigStore) error {
	var names []string
	for name, providerCfg := range cfg.Config().Providers.Seq2() {
		if providerCfg.Disable {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		return errors.New("no providers are configured; see `atlas models` for configured providers")
	}

	var failed int
	for _, name := range names {
		if err := testOneProvider(cmd, cfg, name); err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d providers failed", failed, len(names))
	}
	return nil
}

// testOneProvider checks a single provider and prints its result. It
// returns the connection error (if any) so the caller can count failures,
// but the result is always printed regardless of what the caller does with
// that error -- a batch run should show every provider's outcome, not stop
// at the first failure.
func testOneProvider(cmd *cobra.Command, cfg *config.ConfigStore, name string) error {
	providerCfg, ok := cfg.Config().Providers.Get(name)
	if !ok {
		err := fmt.Errorf("no provider named %q is configured; see `atlas models` for configured providers", name)
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", name, err)
		return err
	}

	if err := providerCfg.TestConnection(cfg.Resolver()); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: failed: %s\n", name, err)
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", name)
	return nil
}

func runProviderList(cmd *cobra.Command, _ []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init("", dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	return listProviders(cmd, cfg)
}

// listProviders is split out from runProviderList for the same reason
// testAllProviders is split from runProviderTest: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func listProviders(cmd *cobra.Command, cfg *config.ConfigStore) error {
	var names []string
	for name := range cfg.Config().Providers.Seq2() {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No providers configured.")
		return nil
	}

	out := cmd.OutOrStdout()
	for _, name := range names {
		providerCfg, _ := cfg.Config().Providers.Get(name)

		status := "no API key"
		if providerCfg.APIKey != "" {
			if resolved, err := cfg.Resolve(providerCfg.APIKey); err == nil && resolved != "" {
				status = "API key set"
			} else {
				status = "API key not resolved"
			}
		}
		if providerCfg.Disable {
			status = "disabled"
		}

		fmt.Fprintf(out, "%s (%s): %d models, %s\n", name, providerCfg.Type, len(providerCfg.Models), status)
	}
	return nil
}
