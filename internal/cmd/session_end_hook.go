package cmd

import (
	"context"
	"log/slog"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/hooks"
)

// fireSessionEndHook runs configured EventSessionEnd hooks after a session
// has been deleted (`session delete`, or a prune sweep) -- the one point in
// this fork's CLI where a session's lifetime has an explicit, unambiguous
// end. It cannot block the deletion, which has already happened by the
// time this runs; it exists for cleanup or archival, not approval. A hook
// execution error is logged and otherwise ignored, since a broken script
// must not turn a successful deletion into a failed command.
func fireSessionEndHook(ctx context.Context, cfg *config.ConfigStore, sessionID string) {
	configured := cfg.Config().Hooks[hooks.EventSessionEnd]
	if len(configured) == 0 {
		return
	}
	runner := hooks.NewRunner(configured, cfg.WorkingDir(), cfg.WorkingDir())
	if _, err := runner.RunSessionEvent(ctx, hooks.EventSessionEnd, sessionID); err != nil {
		slog.Warn("SessionEnd hook execution error", "session_id", sessionID, "error", err)
	}
}
