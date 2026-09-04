package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/app"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/backend"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/proto"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// ListByParent and IsAgentToolSession extend stubSessions (defined in
// sessions_isbusy_test.go) with just enough behaviour for the Agent Hub
// handler test below: filtering children by parent, and recognising the
// "$$"-joined agent-tool session ID format used by the real service.
func (s *stubSessions) ListByParent(_ context.Context, parentID string) ([]session.Session, error) {
	var out []session.Session
	for _, sess := range s.all {
		if sess.ParentSessionID == parentID {
			out = append(out, sess)
		}
	}
	return out, nil
}

// IsAgentToolSession mirrors session.service's real classification: an
// agent-tool session ID is a literal "$$" join of a messageID and a
// toolCallID (see CreateAgentToolSessionID).
func (s *stubSessions) IsAgentToolSession(sessionID string) bool {
	return strings.Count(sessionID, "$$") == 1
}

func TestAgentHubListsAgentToolChildrenWithBusyStatus(t *testing.T) {
	t.Parallel()

	const parentID = "parent-1"
	const busyChild = "msg-1$$call-1"
	const idleChild = "msg-2$$call-2"
	const nonAgentToolChild = "plain-child"

	b := backend.New(context.Background(), nil, nil)
	wsID := uuid.New().String()
	coord := &stubCoordinator{busy: map[string]bool{busyChild: true}}
	a := &app.App{AgentCoordinator: coord}
	a.Sessions = &stubSessions{all: []session.Session{
		{ID: parentID, Title: "Parent"},
		{ID: busyChild, ParentSessionID: parentID, Title: "Busy sub-agent", Cost: 0.05, MessageCount: 4},
		{ID: idleChild, ParentSessionID: parentID, Title: "Finished sub-agent", Cost: 0.02, MessageCount: 2},
		{ID: nonAgentToolChild, ParentSessionID: parentID, Title: "Not an agent-tool session"},
	}}

	ws := &backend.Workspace{ID: wsID, Path: t.TempDir(), App: a}
	backend.InsertWorkspaceForTest(b, ws)

	c := &controllerV1{backend: b, server: &Server{backend: b}}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/workspaces/"+wsID+"/sessions/"+parentID+"/agenthub", nil)
	req.SetPathValue("id", wsID)
	req.SetPathValue("sid", parentID)
	rec := httptest.NewRecorder()
	c.handleGetWorkspaceSessionAgentHub(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got []proto.AgentHubEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2, "the non-agent-tool child must be excluded")

	byID := make(map[string]proto.AgentHubEntry, len(got))
	for _, e := range got {
		byID[e.SessionID] = e
	}

	busy, ok := byID[busyChild]
	require.True(t, ok)
	require.True(t, busy.Busy)
	require.Equal(t, "Busy sub-agent", busy.Title)
	require.InDelta(t, 0.05, busy.Cost, 1e-9)
	require.Equal(t, int64(4), busy.MessageCount)

	idle, ok := byID[idleChild]
	require.True(t, ok)
	require.False(t, idle.Busy, "a finished sub-agent must not be reported as busy")
	require.Equal(t, "Finished sub-agent", idle.Title)
}

func TestAgentHubUnknownWorkspaceErrors(t *testing.T) {
	t.Parallel()

	b := backend.New(context.Background(), nil, nil)
	c := &controllerV1{backend: b, server: &Server{backend: b}}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/workspaces/does-not-exist/sessions/s1/agenthub", nil)
	req.SetPathValue("id", "does-not-exist")
	req.SetPathValue("sid", "s1")
	rec := httptest.NewRecorder()
	c.handleGetWorkspaceSessionAgentHub(rec, req)
	require.NotEqual(t, http.StatusOK, rec.Code)
}
