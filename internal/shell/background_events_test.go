package shell

import (
	"context"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/pubsub"
	"github.com/stretchr/testify/require"
)

// collectJobEvents drains n events (or times out) from the manager's
// broker.
func collectJobEvents(t *testing.T, ch <-chan pubsub.Event[JobEvent], n int) []JobEvent {
	t.Helper()
	events := make([]JobEvent, 0, n)
	for range n {
		select {
		case e := <-ch:
			events = append(events, e.Payload)
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for job event %d/%d (got %d)", len(events)+1, n, len(events))
		}
	}
	return events
}

func TestBackgroundShellManager_JobEventsAndListInfo(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	workingDir := t.TempDir()
	manager := newBackgroundShellManager()

	subCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	ch := manager.Subscribe(subCtx)

	before := time.Now()
	bgShell, err := manager.Start(ctx, workingDir, nil, "echo done", "")
	require.NoError(t, err)

	events := collectJobEvents(t, ch, 2)
	require.Equal(t, JobEventStarted, events[0].Type)
	require.Equal(t, bgShell.ID, events[0].Info.ID)
	require.False(t, events[0].Info.Done)
	require.False(t, events[0].Info.StartedAt.Before(before))

	require.Equal(t, JobEventCompleted, events[1].Type)
	require.Equal(t, bgShell.ID, events[1].Info.ID)
	require.True(t, events[1].Info.Done)
	require.Equal(t, JobStatusDone, events[1].Info.Status)

	infos := manager.ListInfo()
	require.Len(t, infos, 1)
	require.Equal(t, bgShell.ID, infos[0].ID)
	require.True(t, infos[0].Done)
	require.Equal(t, JobStatusDone, infos[0].Status)

	manager.Remove(bgShell.ID)
}

func TestBackgroundShellManager_KillPublishesKilledStatus(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	workingDir := t.TempDir()
	manager := newBackgroundShellManager()

	subCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	ch := manager.Subscribe(subCtx)

	bgShell, err := manager.Start(ctx, workingDir, nil, "sleep 10", "")
	require.NoError(t, err)

	// Drain the start event before killing.
	started := collectJobEvents(t, ch, 1)
	require.Equal(t, JobEventStarted, started[0].Type)

	require.NoError(t, manager.Kill(bgShell.ID))

	completed := collectJobEvents(t, ch, 1)
	require.Equal(t, JobEventCompleted, completed[0].Type)
	require.Equal(t, JobStatusKilled, completed[0].Info.Status)

	// Kill already removed it from the manager (matching the existing
	// Kill contract exercised by TestBackgroundShellManager_Kill), so no
	// further event should follow.
	select {
	case e := <-ch:
		t.Fatalf("unexpected extra job event: %+v", e.Payload)
	case <-time.After(200 * time.Millisecond):
	}
}
