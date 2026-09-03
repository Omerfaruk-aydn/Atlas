package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/event"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/spf13/cobra"
)

var sessionCompactJSON bool

var sessionCompactCmd = &cobra.Command{
	Use:   "compact <id>",
	Short: "Summarize a session's history now, instead of waiting for auto-compaction",
	Long: "Runs the same summarization the agent triggers automatically once a session's context " +
		"fills up, on demand. Useful right before starting a long task on an existing session, so it " +
		"begins with a small summary instead of the full transcript. This is a real model call against " +
		"the session's configured provider, not a local operation. ID can be a UUID, full hash, or hash " +
		"prefix, as shown by `atlas session list`. Use --json for machine-readable output.",
	Args: cobra.ExactArgs(1),
	RunE: runSessionCompact,
}

func init() {
	sessionCompactCmd.Flags().BoolVar(&sessionCompactJSON, "json", false, "output in JSON format")
	sessionCmd.AddCommand(sessionCompactCmd)
}

// jsonCompactResult is compactSession's wire form for --json.
type jsonCompactResult struct {
	SessionID     string `json:"session_id"`
	SessionIDHash string `json:"session_id_hash"`
	Compacted     bool   `json:"compacted"`
	Error         string `json:"error,omitempty"`
}

func runSessionCompact(cmd *cobra.Command, args []string) error {
	event.SetNonInteractive(true)
	ctx := cmd.Context()

	if useClientServer() {
		c, ws, cleanup, err := connectToServer(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		if !ws.Config.IsConfigured() {
			return fmt.Errorf("no providers configured - please run 'atlas' to set up a provider interactively")
		}

		clientWs := workspace.NewClientWorkspace(c, *ws)
		if err := clientWs.InitCoderAgentNonInteractive(ctx); err != nil {
			return fmt.Errorf("failed to initialize agent: %w", err)
		}

		sess, err := resolveSessionByID(ctx, c, ws.ID, args[0])
		if err != nil {
			return err
		}

		return compactSession(ctx, clientWs, sess.ID, cmd.OutOrStdout(), sessionCompactJSON)
	}

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

	return compactSession(ctx, appWs, sess.ID, cmd.OutOrStdout(), sessionCompactJSON)
}

// compactSession runs the actual summarization against a session ID already
// resolved from the (UUID/hash/prefix) argument the user gave, and reports
// the result. Shared by the client/server and local branches above, and the
// seam a fake workspace.Workspace exercises in tests -- resolving that
// argument requires either a live server or a fully booted local app, which
// this package does not otherwise unit test against (see run.go).
//
// A summarization failure is still reported as JSON (with compacted: false
// and the error text) rather than only as a bare error, so a script parsing
// --json output always gets a well-formed record; the error is also
// returned either way so the process exit status reflects it.
func compactSession(ctx context.Context, ws workspace.Workspace, sessionID string, out io.Writer, jsonOut bool) error {
	summarizeErr := ws.AgentSummarize(ctx, sessionID)

	if jsonOut {
		result := jsonCompactResult{
			SessionID:     sessionID,
			SessionIDHash: session.HashID(sessionID),
			Compacted:     summarizeErr == nil,
		}
		if summarizeErr != nil {
			result.Error = summarizeErr.Error()
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	} else if summarizeErr == nil {
		fmt.Fprintf(out, "Session %s compacted.\n", session.HashID(sessionID))
	}

	if summarizeErr != nil {
		return fmt.Errorf("failed to compact session: %w", summarizeErr)
	}
	return nil
}
