package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect the effective configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the merged configuration this workspace runs with",
	Long: "Print the configuration this workspace actually runs with -- every file, environment, " +
		"and default merged into one document. Secrets are redacted; use the config files themselves " +
		"if you need the real values.",
	Args: cobra.NoArgs,
	RunE: runConfigShow,
}

var configPathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "List the config files this workspace merges",
	Long: "List every config file location consulted for this workspace, in the order they are merged -- " +
		"later files win on conflict -- and whether each one exists.",
	Args: cobra.NoArgs,
	RunE: runConfigPaths,
}

func init() {
	configCmd.AddCommand(configPathsCmd)
	configCmd.AddCommand(configShowCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
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
	return showConfig(cmd, cfg)
}

// showConfig is split out from runConfigShow for the same reason
// listProviders is split from runProviderList: a test can build a
// *config.ConfigStore directly but cannot inject one into a function that
// calls config.Init itself.
func showConfig(cmd *cobra.Command, cfg *config.ConfigStore) error {
	raw, err := json.Marshal(cfg.Config())
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decoding config: %w", err)
	}

	out, err := json.MarshalIndent(redactSecrets(doc), "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return err
}

// Redacted is what a secret's value is replaced with. It is a fixed string
// rather than a length-preserving mask so the output never hints at how long
// the real value is.
const Redacted = "[redacted]"

// secretKeys are config keys whose value is a secret wherever it appears.
// Matching is exact rather than by substring: "token" as a substring would
// also redact every model's max_tokens, which is not a secret and is one of
// the more useful things to read out of an effective config.
var secretKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"api_token":     {},
	"apikey":        {},
	"auth_token":    {},
	"authorization": {},
	"client_secret": {},
	"oauth":         {},
	"password":      {},
	"refresh_token": {},
	"secret":        {},
	"token":         {},
}

// secretMaps are config keys holding a map whose *values* are all secrets --
// the keys stay, since knowing that ANTHROPIC_API_KEY is set is the useful
// half and is not itself sensitive.
var secretMaps = []string{"env", "headers"}

func isSecretKey(key string) bool {
	_, ok := secretKeys[strings.ToLower(key)]
	return ok
}

// redactSecrets walks decoded config JSON and replaces every secret value it
// finds. It matches on key name at any depth rather than on a list of known
// paths, so a provider or MCP server added later is covered without anyone
// remembering to extend this.
func redactSecrets(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for key, val := range t {
			switch {
			case isSecretKey(key):
				if val == nil {
					out[key] = nil
					continue
				}
				out[key] = Redacted
			case isSecretMapKey(key):
				out[key] = redactMapValues(val)
			default:
				out[key] = redactSecrets(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactSecrets(val)
		}
		return out
	default:
		return v
	}
}

func isSecretMapKey(key string) bool {
	for _, s := range secretMaps {
		if strings.EqualFold(key, s) {
			return true
		}
	}
	return false
}

func redactMapValues(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return redactSecrets(v)
	}
	out := make(map[string]any, len(m))
	for key := range m {
		out[key] = Redacted
	}
	return out
}

func runConfigPaths(cmd *cobra.Command, _ []string) error {
	cwd, err := ResolveCwd(cmd)
	if err != nil {
		return err
	}
	return printConfigPaths(cmd.OutOrStdout(), config.ProjectConfigs(cwd))
}

// printConfigPaths lists the candidate config files in merge order. The
// order is what makes this worth printing at all: a setting in a later file
// wins, and that is the question people actually have when two files
// disagree.
func printConfigPaths(out io.Writer, paths []string) error {
	for _, path := range paths {
		state := "missing"
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			state = "present"
		}
		fmt.Fprintf(out, "[%s] %s\n", state, home.Short(path))
	}
	fmt.Fprintln(out, "\nMerged in this order; a setting in a later file wins.")
	return nil
}
