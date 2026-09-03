package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/spf13/cobra"
)

var doctorJSON bool

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that this workspace is set up to run",
	Long: "Check the things a session needs before it starts: a writable data directory, a working " +
		"database, a configured provider and model, and the commands the configured MCP and LSP " +
		"servers are supposed to run. Reports what is wrong rather than fixing it. " +
		"Use --json for machine-readable output.",
	Args: cobra.NoArgs,
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "output in JSON format")
	rootCmd.AddCommand(doctorCmd)
}

// checkStatus is how a single check came out. A warning is something a
// session can start without; a failure is not.
type checkStatus int

const (
	statusOK checkStatus = iota
	statusWarn
	statusFail
)

func (s checkStatus) String() string {
	switch s {
	case statusWarn:
		return "warn"
	case statusFail:
		return "fail"
	default:
		return "ok"
	}
}

type checkResult struct {
	Name   string
	Status checkStatus
	Detail string
}

func runDoctor(cmd *cobra.Command, _ []string) error {
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
	return reportDiagnostics(cmd, cfg)
}

// reportDiagnostics is split out from runDoctor for the same reason
// listProviders is split from runProviderList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func reportDiagnostics(cmd *cobra.Command, cfg *config.ConfigStore) error {
	return printChecks(cmd.OutOrStdout(), diagnose(cmd.Context(), cfg))
}

// jsonCheckResult is checkResult's wire form: Status renders as its string
// name rather than the underlying int, since a consumer that is not this
// package has no reason to know the iota ordering.
type jsonCheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// printChecks writes the report and turns any failure into an error, so a
// script can act on the exit status instead of parsing the output.
func printChecks(out io.Writer, results []checkResult) error {
	failures := countFailures(results)

	if doctorJSON {
		wire := make([]jsonCheckResult, len(results))
		for i, r := range results {
			wire[i] = jsonCheckResult{Name: r.Name, Status: r.Status.String(), Detail: r.Detail}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(wire); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			fmt.Fprintf(out, "[%s] %s: %s\n", r.Status, r.Name, r.Detail)
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d check(s) failed", failures)
	}
	return nil
}

func countFailures(results []checkResult) int {
	var failures int
	for _, r := range results {
		if r.Status == statusFail {
			failures++
		}
	}
	return failures
}

func diagnose(ctx context.Context, cfg *config.ConfigStore) []checkResult {
	results := []checkResult{
		checkDataDirectory(cfg),
		checkDatabase(ctx, cfg),
		checkProviders(cfg),
		checkModels(cfg),
	}
	results = append(results, checkMCPCommands(cfg)...)
	results = append(results, checkLSPCommands(cfg)...)
	if r := checkBrowser(cfg); r != nil {
		results = append(results, *r)
	}
	return results
}

// browserCandidates are the binary names checkBrowser looks for on PATH
// when the user hasn't set an explicit executable_path.
var browserCandidates = []string{
	"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome", "msedge",
}

// checkBrowser reports whether the browser tool, if turned on, has a
// Chrome/Chromium/Edge binary to launch. It reports nothing when the tool
// is off, since it is opt-in and a missing browser is irrelevant until
// then. A miss is a warning, not a failure: chromedp also probes common
// install directories beyond PATH, so this check can under-report a
// browser that is actually there -- the tool itself gives a clearer error
// the first time it tries to launch one.
func checkBrowser(cfg *config.ConfigStore) *checkResult {
	b := cfg.Config().Tools.Browser
	if !b.IsEnabled() {
		return nil
	}

	if b.ExecutablePath != "" {
		if _, err := os.Stat(b.ExecutablePath); err != nil {
			return &checkResult{"browser", statusFail, fmt.Sprintf("executable_path %s: %s", b.ExecutablePath, err)}
		}
		return &checkResult{"browser", statusOK, b.ExecutablePath}
	}

	for _, name := range browserCandidates {
		if path, err := exec.LookPath(name); err == nil {
			return &checkResult{"browser", statusOK, path}
		}
	}
	return &checkResult{"browser", statusWarn, "no Chrome/Chromium/Edge found on PATH; set tools.browser.executable_path if one is installed elsewhere"}
}

func checkDataDirectory(cfg *config.ConfigStore) checkResult {
	dir := cfg.Config().Options.DataDirectory
	if dir == "" {
		return checkResult{"data directory", statusFail, "not configured"}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return checkResult{"data directory", statusFail, fmt.Sprintf("%s: %s", dir, err)}
	}

	// Writability is what actually matters, and on Windows the mode bits
	// do not tell us -- so write something and take it away again.
	probe := filepath.Join(dir, ".atlas-doctor")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return checkResult{"data directory", statusFail, fmt.Sprintf("%s is not writable: %s", dir, err)}
	}
	_ = os.Remove(probe)
	return checkResult{"data directory", statusOK, dir}
}

func checkDatabase(ctx context.Context, cfg *config.ConfigStore) checkResult {
	dir := cfg.Config().Options.DataDirectory
	if dir == "" {
		return checkResult{"database", statusFail, "no data directory to open it in"}
	}

	conn, err := db.Connect(ctx, dir)
	if err != nil {
		return checkResult{"database", statusFail, err.Error()}
	}
	defer func() { _ = db.Release(dir) }()

	if err := conn.PingContext(ctx); err != nil {
		return checkResult{"database", statusFail, err.Error()}
	}
	return checkResult{"database", statusOK, filepath.Join(dir, "atlas.db")}
}

func checkProviders(cfg *config.ConfigStore) checkResult {
	var enabled, withKey int
	for _, providerCfg := range cfg.Config().Providers.Seq2() {
		if providerCfg.Disable {
			continue
		}
		enabled++
		if providerCfg.APIKey == "" {
			continue
		}
		if resolved, err := cfg.Resolve(providerCfg.APIKey); err == nil && resolved != "" {
			withKey++
		}
	}

	switch {
	case enabled == 0:
		return checkResult{"providers", statusFail, "none configured; run `atlas login` or set an API key"}
	case withKey == 0:
		// Not a failure: a local endpoint needs no key at all.
		return checkResult{"providers", statusWarn, fmt.Sprintf("%d configured, none with a resolvable API key", enabled)}
	default:
		return checkResult{"providers", statusOK, fmt.Sprintf("%d configured, %d with an API key", enabled, withKey)}
	}
}

func checkModels(cfg *config.ConfigStore) checkResult {
	models := cfg.Config().Models
	large, hasLarge := models[config.SelectedModelTypeLarge]
	if !hasLarge || large.Model == "" {
		return checkResult{"models", statusFail, "no large model selected; run `atlas models`"}
	}
	small, hasSmall := models[config.SelectedModelTypeSmall]
	if !hasSmall || small.Model == "" {
		return checkResult{"models", statusWarn, fmt.Sprintf("large=%s, no small model selected", large.Model)}
	}
	return checkResult{"models", statusOK, fmt.Sprintf("large=%s, small=%s", large.Model, small.Model)}
}

func checkMCPCommands(cfg *config.ConfigStore) []checkResult {
	var names []string
	for name := range cfg.Config().MCP {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []checkResult
	for _, name := range names {
		m := cfg.Config().MCP[name]
		if m.Disabled || m.Type != config.MCPStdio {
			continue
		}
		out = append(out, commandCheck("mcp "+name, m.Command, cfg))
	}
	return out
}

func checkLSPCommands(cfg *config.ConfigStore) []checkResult {
	var names []string
	for name := range cfg.Config().LSP {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []checkResult
	for _, name := range names {
		l := cfg.Config().LSP[name]
		if l.Disabled {
			continue
		}
		out = append(out, commandCheck("lsp "+name, l.Command, cfg))
	}
	return out
}

// commandCheck reports whether a configured server's command can actually be
// run. A missing binary is a warning rather than a failure: a session starts
// fine without one server, it just loses that server's tools.
func commandCheck(name, command string, cfg *config.ConfigStore) checkResult {
	if command == "" {
		return checkResult{name, statusWarn, "no command configured"}
	}

	resolved := command
	if r, err := cfg.Resolve(command); err == nil && r != "" {
		resolved = r
	}

	path, err := exec.LookPath(resolved)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return checkResult{name, statusWarn, fmt.Sprintf("%s not found on PATH", resolved)}
		}
		return checkResult{name, statusWarn, fmt.Sprintf("%s: %s", resolved, err)}
	}
	return checkResult{name, statusOK, path}
}
