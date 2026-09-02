package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

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

var mcpTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Check that configured MCP servers are reachable",
	Long: "Check that a configured MCP server is reachable, without starting a session: a stdio server's " +
		"command must resolve on PATH, an HTTP/SSE server's URL must accept a connection. This is a basic " +
		"reachability check, not a full MCP protocol handshake -- a server that answers with an HTTP error " +
		"still counts as reachable, since only a live server produces one at all. " +
		"[name] is the server's key in the config file's mcp map. Omit it to check every configured, enabled server.",
	Args: cobra.MaximumNArgs(1),
	RunE: runMCPTest,
}

func init() {
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpTestCmd)
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

func runMCPTest(cmd *cobra.Command, args []string) error {
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

	if len(args) == 1 {
		return testOneMCPServer(cmd, cfg, args[0])
	}
	return testAllMCPServers(cmd, cfg)
}

// testAllMCPServers checks every configured, enabled server and reports each
// one's result, split out from runMCPTest so it can be driven against a
// *config.ConfigStore built directly in a test -- runMCPTest itself always
// builds its own via config.Init, which a test cannot inject a fake into.
func testAllMCPServers(cmd *cobra.Command, cfg *config.ConfigStore) error {
	mcps := cfg.Config().MCP
	names := make([]string, 0, len(mcps))
	for name, m := range mcps {
		if m.Disabled {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		return errors.New("no MCP servers are configured; see `atlas mcp list`")
	}

	var failed int
	for _, name := range names {
		if err := testOneMCPServer(cmd, cfg, name); err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d MCP servers failed", failed, len(names))
	}
	return nil
}

// testOneMCPServer checks a single server and prints its result. It returns
// the connection error (if any) so the caller can count failures, but the
// result is always printed regardless of what the caller does with that
// error -- a batch run should show every server's outcome, not stop at the
// first failure.
func testOneMCPServer(cmd *cobra.Command, cfg *config.ConfigStore, name string) error {
	m, ok := cfg.Config().MCP[name]
	if !ok {
		err := fmt.Errorf("no MCP server named %q is configured; see `atlas mcp list`", name)
		fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", name, err)
		return err
	}

	if err := testMCPReachability(cmd.Context(), m, cfg.Resolver()); err != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%s: failed: %s\n", name, err)
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", name)
	return nil
}

// testMCPReachability performs the basic check described in mcpTestCmd's
// help: a stdio command resolves on PATH, an HTTP/SSE URL accepts a
// connection. It never speaks the MCP protocol itself, so it cannot tell a
// well-behaved server from a misconfigured endpoint that merely answers --
// callers should read "ok" as "reachable," not "working."
func testMCPReachability(ctx context.Context, m config.MCPConfig, resolver config.VariableResolver) error {
	if m.Type == config.MCPStdio {
		if m.Command == "" {
			return errors.New("no command configured")
		}
		resolved, err := resolver.ResolveValue(m.Command)
		if err != nil {
			return fmt.Errorf("resolving command: %w", err)
		}
		if _, err := exec.LookPath(resolved); err != nil {
			return fmt.Errorf("command not found: %w", err)
		}
		return nil
	}

	if m.URL == "" {
		return errors.New("no url configured")
	}
	url, err := resolver.ResolveValue(m.URL)
	if err != nil {
		return fmt.Errorf("resolving url: %w", err)
	}

	timeout := 10 * time.Second
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	headers, err := m.ResolvedHeaders(resolver)
	if err != nil {
		return fmt.Errorf("resolving headers: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer resp.Body.Close()
	return nil
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
