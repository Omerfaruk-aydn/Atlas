package cmd

import (
	"fmt"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/spf13/cobra"
)

var sessionCompactCmd = &cobra.Command{
	Use:   "compact <id>",
	Short: "Summarize a session's history now, instead of waiting for auto-compaction",
	Long: "Runs the same summarization the agent triggers automatically once a session's context " +
		"fills up, on demand. Useful right before starting a long task on an existing session, so it " +
		"begins with a small summary instead of the full transcript. This is a real model call against " +
		"the session's configured provider, not a local operation. ID can be a UUID, full hash, or hash " +
		"prefix, as shown by `atlas session list`.",
	Args: cobra.ExactArgs(1),
	RunE: runSessionCompact,
}

func init() {
	sessionCmd.AddCommand(sessionCompactCmd)
}

func runSessionCompact(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)
	ctx := cmd.Context()

	ws, cleanup, err := setupLocalWorkspace(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	if !ws.Config().IsConfigured() {
		return fmt.Errorf("no providers configured - please run 'atlas' to set up a provider interactively")
	}

	appWs := ws.(*workspace.AppWorkspace)

	sess, err := resolveSessionID(ctx, appWs.App().Sessions, args[0])
	if err != nil {
		return err
	}

	if err := appWs.AgentSummarize(ctx, sess.ID); err != nil {
		return fmt.Errorf("failed to compact session: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Session %s compacted.\n", session.HashID(sess.ID))
	return nil
}
