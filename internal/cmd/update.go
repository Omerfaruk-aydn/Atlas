package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/update"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/version"
	"github.com/spf13/cobra"
)

// updateCmd upgrades the npm-distributed wrapper package to the latest
// version published on the npm registry. It exists so the TUI can offer
// "press U to update" without shelling out to npm from inside the chat
// loop; the wrapper, in turn, downloads and replaces the platform binary
// via its own postinstall step.
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Upgrade Atlas Agent to the latest version",
	Long: `Check the npm registry for a newer version of @atlas-coder/atlas-agent
and install it globally. The postinstall step in the npm wrapper will replace
the platform binary in <npm prefix>/bin with the matching release.

If you installed Atlas Agent via 'go install' instead, run:

  go install github.com/Omerfaruk-aydn/Atlas-Agent@latest`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()

		fmt.Printf("Current version: v%s\n", version.Version)

		info, err := update.Check(ctx, version.Version, update.Default)
		if err != nil {
			return fmt.Errorf("check for updates: %w", err)
		}
		if info.IsDevelopment() {
			fmt.Println("Skipping update check: this appears to be a development build.")
			return nil
		}
		if !info.Available() {
			fmt.Printf("Already on the latest version (v%s).\n", info.Current)
			return nil
		}

		fmt.Printf("Updating v%s → v%s ...\n", info.Current, info.Latest)

		// The wrapper package is what npm distributes; the binary it
		// installs is replaced by the postinstall script, so the user
		// gets a self-contained upgrade without any extra steps.
		installCmd := exec.CommandContext(ctx, "npm", "install", "-g",
			"@atlas-coder/atlas-agent@latest")
		installCmd.Stdin = os.Stdin
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		installCmd.Env = append(os.Environ(),
			// npm uses the prefix; let the user override via env if they
			// installed to a non-default location.
			"NPM_CONFIG_UPDATE_NOTIFIER=false",
		)
		if err := installCmd.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}

		fmt.Printf("\nUpdated to v%s. Restart any running Atlas Agent sessions.\n", info.Latest)
		return nil
	},
}
