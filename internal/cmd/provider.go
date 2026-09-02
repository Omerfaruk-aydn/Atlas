package cmd

import (
	"fmt"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
)

var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Inspect configured providers",
}

var providerTestCmd = &cobra.Command{
	Use:   "test <name>",
	Short: "Check that a configured provider's credentials work",
	Long: "Check that a configured provider's credentials work, without starting a session. " +
		"Makes one request to the provider (e.g. listing models) and reports whether it succeeded. " +
		"<name> is the provider ID as it appears in the config file's providers map, or in `atlas models`.",
	Args: cobra.ExactArgs(1),
	RunE: runProviderTest,
}

func init() {
	providerCmd.AddCommand(providerTestCmd)
	rootCmd.AddCommand(providerCmd)
}

func runProviderTest(cmd *cobra.Command, args []string) error {
	dataDir, _ := cmd.Flags().GetString("data-dir")
	debug, _ := cmd.Flags().GetBool("debug")

	cfg, err := config.Init("", dataDir, debug)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	name := args[0]
	providerCfg, ok := cfg.Config().Providers.Get(name)
	if !ok {
		return fmt.Errorf("no provider named %q is configured; see `atlas models` for configured providers", name)
	}

	if err := providerCfg.TestConnection(cfg.Resolver()); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", name)
	return nil
}
