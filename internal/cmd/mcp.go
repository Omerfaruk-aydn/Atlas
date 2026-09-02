package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Inspect configured MCP servers",
}

var mcpListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured MCP servers",
	Long: "List the MCP servers this workspace has configured, their transport, and whether each is enabled. " +
		"Does not start or contact any server.",
	Args: cobra.NoArgs,
	RunE: runMCPList,
}

func init() {
	mcpCmd.AddCommand(mcpListCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPList(cmd *cobra.Command, _ []string) error {
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
	return listMCPServers(cmd, cfg)
}

// listMCPServers is split out from runMCPList for the same reason
// listProviders is split from runProviderList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func listMCPServers(cmd *cobra.Command, cfg *config.ConfigStore) error {
	mcps := cfg.Config().MCP
	names := make([]string, 0, len(mcps))
	for name := range mcps {
		names = append(names, name)
	}
	sort.Strings(names)

	out := cmd.OutOrStdout()
	if len(names) == 0 {
		fmt.Fprintln(out, "No MCP servers configured.")
		return nil
	}

	for _, name := range names {
		m := mcps[name]
		status := "enabled"
		if m.Disabled {
			status = "disabled"
		}
		fmt.Fprintf(out, "%s (%s, %s)\n  %s\n", name, m.Type, status, mcpEndpoint(m))
		if tools := mcpToolFilter(m); tools != "" {
			fmt.Fprintf(out, "  %s\n", tools)
		}
	}
	return nil
}

// mcpEndpoint describes where a server lives: the URL for the HTTP
// transports, the command line for stdio. Env values are deliberately not
// printed -- they routinely hold API keys.
func mcpEndpoint(m config.MCPConfig) string {
	if m.Type == config.MCPStdio {
		if m.Command == "" {
			return "(no command configured)"
		}
		return strings.TrimSpace(m.Command + " " + strings.Join(m.Args, " "))
	}
	if m.URL == "" {
		return "(no url configured)"
	}
	return m.URL
}

func mcpToolFilter(m config.MCPConfig) string {
	var parts []string
	if len(m.EnabledTools) > 0 {
		parts = append(parts, "only: "+strings.Join(m.EnabledTools, ", "))
	}
	if len(m.DisabledTools) > 0 {
		parts = append(parts, "disabled: "+strings.Join(m.DisabledTools, ", "))
	}
	return strings.Join(parts, "; ")
}
