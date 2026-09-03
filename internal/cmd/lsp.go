package cmd

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
)

var lspListJSON bool

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Inspect configured LSP servers",
}

var lspListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured LSP servers",
	Long: "List the LSP servers this workspace has configured, the file types they handle, and whether " +
		"each is enabled and resolvable on PATH. Does not start any server. Use --json for machine-readable output.",
	Args: cobra.NoArgs,
	RunE: runLSPList,
}

func init() {
	lspListCmd.Flags().BoolVar(&lspListJSON, "json", false, "output in JSON format")
	lspCmd.AddCommand(lspListCmd)
	rootCmd.AddCommand(lspCmd)
}

func runLSPList(cmd *cobra.Command, _ []string) error {
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
	return listLSPServers(cmd, cfg)
}

// jsonLSPServer is one LSP server's wire form for --json.
type jsonLSPServer struct {
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Command   string   `json:"command"`
	FileTypes []string `json:"file_types,omitempty"`
}

// listLSPServers is split out from runLSPList for the same reason
// listMCPServers is split from runMCPList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func listLSPServers(cmd *cobra.Command, cfg *config.ConfigStore) error {
	lsps := cfg.Config().LSP
	names := make([]string, 0, len(lsps))
	for name := range lsps {
		names = append(names, name)
	}
	sort.Strings(names)

	out := cmd.OutOrStdout()

	if lspListJSON {
		servers := make([]jsonLSPServer, 0, len(names))
		for _, name := range names {
			l := lsps[name]
			status := "disabled"
			if !l.Disabled {
				status = lspCommandStatus(l, cfg)
			}
			servers = append(servers, jsonLSPServer{
				Name:      name,
				Status:    status,
				Command:   lspCommandLine(l),
				FileTypes: l.FileTypes,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(servers)
	}

	if len(names) == 0 {
		fmt.Fprintln(out, "No LSP servers configured.")
		return nil
	}

	for _, name := range names {
		l := lsps[name]
		status := "enabled"
		if l.Disabled {
			status = "disabled"
		} else {
			status = lspCommandStatus(l, cfg)
		}
		fmt.Fprintf(out, "%s (%s)\n  %s\n", name, status, lspCommandLine(l))
		if len(l.FileTypes) > 0 {
			fmt.Fprintf(out, "  filetypes: %s\n", strings.Join(l.FileTypes, ", "))
		}
	}
	return nil
}

// lspCommandStatus reports whether an enabled server's command can actually
// be run, the same check doctor makes, so this list explains at a glance
// which servers a session will actually get tools from.
func lspCommandStatus(l config.LSPConfig, cfg *config.ConfigStore) string {
	if l.Command == "" {
		return "enabled, no command configured"
	}
	resolved := l.Command
	if r, err := cfg.Resolve(l.Command); err == nil && r != "" {
		resolved = r
	}
	if _, err := exec.LookPath(resolved); err != nil {
		return "enabled, not found on PATH"
	}
	return "enabled"
}

func lspCommandLine(l config.LSPConfig) string {
	if l.Command == "" {
		return "(no command configured)"
	}
	return strings.TrimSpace(l.Command + " " + strings.Join(l.Args, " "))
}
