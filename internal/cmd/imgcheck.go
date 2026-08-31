package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/charmbracelet/crush/internal/config"
	"github.com/spf13/cobra"
)

// imgcheckCmd explains why attaching an image is allowed or refused. The
// editor's check is a single boolean three lookups deep — agent, then selected
// model, then the model's catalog entry — and when it says no there is no way
// to tell from the UI which lookup produced the no. This walks the same path
// and prints each step, in the same working directory and with the same
// config the TUI would load.
func init() {
	imgcheckCmd.Flags().BoolP("verbose", "v", false, "Print the config loader's own diagnostics to stderr")
}

var imgcheckCmd = &cobra.Command{
	Use:    "imgcheck",
	Short:  "Explain whether the selected model accepts image attachments",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()

		cwd, err := ResolveCwd(cmd)
		if err != nil {
			return err
		}
		dataDir, _ := cmd.Flags().GetString("data-dir")
		fmt.Fprintf(out, "working dir : %s\ndata dir    : %s\n", cwd, dataDir)

		fmt.Fprintf(out, "global cfg  : %s\n", config.GlobalConfig())
		fmt.Fprintf(out, "global data : %s\n", config.GlobalConfigData())

		// The loader reports what it skipped and why through slog, and those
		// messages are the only record of a config file that was found but
		// not used. Route them to stderr so a run in the user's own shell
		// shows them.
		verbose, _ := cmd.Flags().GetBool("verbose")
		if verbose {
			slog.SetDefault(slog.New(slog.NewTextHandler(cmd.ErrOrStderr(),
				&slog.HandlerOptions{Level: slog.LevelDebug})))
		}

		store, err := config.Init(cwd, dataDir, false)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg := store.Config()

		// Which files actually contributed, and the environment that decides
		// where they are looked for. Two shells on one machine can disagree
		// about both, and when they do the merged result differs with no
		// other visible sign.
		fmt.Fprintln(out, "\nloaded config files:")
		for _, p := range store.LoadedPaths() {
			fmt.Fprintf(out, "   %s\n", p)
		}
		fmt.Fprintln(out, "env:")
		for _, k := range []string{
			"CRUSH_GLOBAL_CONFIG", "CRUSH_GLOBAL_DATA", "CRUSH_CACHE_DIR",
			"CRUSH_DISABLE_PROVIDER_AUTO_UPDATE", "CRUSH_DISABLE_DEFAULT_PROVIDERS",
			"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
			"LOCALAPPDATA", "USERPROFILE", "HOME",
		} {
			if v, ok := os.LookupEnv(k); ok {
				fmt.Fprintf(out, "   %-34s = %q\n", k, v)
			}
		}
		for _, ev := range os.Environ() {
			if strings.HasPrefix(ev, "CRUSH_") {
				fmt.Fprintf(out, "   (CRUSH_*) %s\n", ev)
			}
		}

		// The merge that decides this flag has two inputs: the catalog entry
		// and the user's override. Printing both is the only way to tell
		// which one the final answer came from.
		if pc, ok := cfg.Providers.Get("minimax"); ok {
			fmt.Fprintf(out, "\nprovider minimax: type=%q models=%d\n", pc.Type, len(pc.Models))
			for _, m := range pc.Models {
				fmt.Fprintf(out, "   %-28s supports_attachments=%v\n", m.ID, m.SupportsImages)
			}
		} else {
			fmt.Fprintln(out, "\nprovider minimax: NOT PRESENT in the merged config")
		}
		fmt.Fprintln(out)

		agentCfg, ok := cfg.Agents[config.AgentCoder]
		fmt.Fprintf(out, "agent %q   : present=%v model=%q\n", config.AgentCoder, ok, agentCfg.Model)
		if !ok {
			fmt.Fprintln(out, "\n=> refused: no coder agent in the config.")
			return nil
		}

		sel := cfg.Models[agentCfg.Model]
		fmt.Fprintf(out, "selected    : provider=%q model=%q\n", sel.Provider, sel.Model)

		model := cfg.GetModelByType(agentCfg.Model)
		if model == nil {
			fmt.Fprintln(out, "\n=> refused: that model is not in the provider's catalog.")
			return nil
		}
		fmt.Fprintf(out, "resolved    : id=%q supports_attachments=%v\n", model.ID, model.SupportsImages)

		if model.SupportsImages {
			fmt.Fprintln(out, "\n=> allowed: the editor should accept image attachments.")
			return nil
		}
		fmt.Fprintf(out, "\n=> refused: the catalog says %q takes no attachments.\n", model.ID)
		fmt.Fprintf(out, "   To override, add the model to the %q provider in your config with:\n", sel.Provider)
		fmt.Fprintln(out, `     { "id": "`+model.ID+`", "supports_attachments": true }`)
		return nil
	},
}
