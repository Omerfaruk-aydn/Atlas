package backend

import (
	"context"

	"github.com/charmbracelet/crush/internal/session"
	"github.com/charmbracelet/crush/internal/shell"
)

// ListBackgroundJobs returns every tracked background shell job. Like
// GetLSPStates, the underlying manager is process-global; workspaceID is
// validated for existence/error consistency with the rest of the API.
func (b *Backend) ListBackgroundJobs(workspaceID string) ([]shell.BackgroundShellInfo, error) {
	if _, err := b.GetWorkspace(workspaceID); err != nil {
		return nil, err
	}
	return shell.GetBackgroundShellManager().ListInfo(), nil
}

// KillBackgroundJob terminates a background shell job by ID.
func (b *Backend) KillBackgroundJob(workspaceID, jobID string) error {
	if _, err := b.GetWorkspace(workspaceID); err != nil {
		return err
	}
	return shell.GetBackgroundShellManager().Kill(jobID)
}

// ListChildSessions returns every session whose parent is sessionID, for
// the sub-agent runs panel.
func (b *Backend) ListChildSessions(ctx context.Context, workspaceID, sessionID string) ([]session.Session, error) {
	ws, err := b.GetWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	return ws.Sessions.ListByParent(ctx, sessionID)
}
