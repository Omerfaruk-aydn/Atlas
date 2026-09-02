package cmd

import (
	"fmt"
	"os"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session/export"
	"github.com/spf13/cobra"
)

var sessionExportOutput string

var sessionExportCmd = &cobra.Command{
	Use:   "export <id>",
	Short: "Export a session as Markdown",
	Long: "Export a session's conversation as Markdown, for saving or sharing outside the TUI. " +
		"Writes to stdout by default; use --output to write to a file instead. " +
		"ID can be a UUID, full hash, or hash prefix.",
	Args: cobra.ExactArgs(1),
	RunE: runSessionExport,
}

func init() {
	sessionExportCmd.Flags().StringVarP(&sessionExportOutput, "output", "o", "", "file to write to (default: stdout)")
	sessionCmd.AddCommand(sessionExportCmd)
}

func runSessionExport(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)

	ctx, svc, cleanup, err := sessionSetup(cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	sess, err := resolveSessionID(ctx, svc.sessions, args[0])
	if err != nil {
		return err
	}

	msgs, err := svc.messages.List(ctx, sess.ID)
	if err != nil {
		return fmt.Errorf("failed to list messages: %w", err)
	}

	doc := export.Markdown(sess, msgs)

	if sessionExportOutput == "" {
		_, err := fmt.Fprint(cmd.OutOrStdout(), doc)
		return err
	}

	if err := os.WriteFile(sessionExportOutput, []byte(doc), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", sessionExportOutput, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Exported to %s\n", sessionExportOutput)
	return nil
}
