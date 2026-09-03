package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

// fakeCompactWorkspace is a workspace.Workspace stub that only implements
// AgentSummarize, the one method compactSession calls -- resolving a
// session ID argument into a workspace.Workspace in the first place needs
// either a live server or a fully booted local app (see runSessionCompact),
// which this package does not otherwise unit test against (see run.go).
// The embedded interface panics on anything else, so a test that reaches
// further than compactSession's own contract fails loudly instead of
// silently no-oping.
type fakeCompactWorkspace struct {
	workspace.Workspace

	summarizeCalls []string
	summarizeErr   error
}

func (w *fakeCompactWorkspace) AgentSummarize(_ context.Context, sessionID string) error {
	w.summarizeCalls = append(w.summarizeCalls, sessionID)
	return w.summarizeErr
}

func TestCompactSessionReportsSuccess(t *testing.T) {
	t.Parallel()

	ws := &fakeCompactWorkspace{}
	var out bytes.Buffer

	err := compactSession(t.Context(), ws, "session-1", &out)
	require.NoError(t, err)
	require.Equal(t, []string{"session-1"}, ws.summarizeCalls)
	require.Equal(t, "Session "+session.HashID("session-1")+" compacted.\n", out.String())
}

func TestCompactSessionWrapsTheUnderlyingError(t *testing.T) {
	t.Parallel()

	ws := &fakeCompactWorkspace{summarizeErr: errors.New("provider unreachable")}
	var out bytes.Buffer

	err := compactSession(t.Context(), ws, "session-1", &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to compact session")
	require.Contains(t, err.Error(), "provider unreachable")
	require.Empty(t, out.String(), "no success line when summarization fails")
}

func TestSessionCompactCommandRequiresExactlyOneArg(t *testing.T) {
	t.Parallel()

	require.Error(t, sessionCompactCmd.Args(sessionCompactCmd, nil))
	require.Error(t, sessionCompactCmd.Args(sessionCompactCmd, []string{"a", "b"}))
	require.NoError(t, sessionCompactCmd.Args(sessionCompactCmd, []string{"a"}))
}
