package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/stretchr/testify/require"
)

func newUsageFixture(t *testing.T, maxSessionCost float64) (fantasy.AgentTool, session.Session) {
	t.Helper()

	conn, err := db.Connect(t.Context(), t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	q := db.New(conn)
	sessions := session.NewService(q, conn)

	sess, err := sessions.Create(t.Context(), "budget check")
	require.NoError(t, err)
	sess.PromptTokens = 1000
	sess.CompletionTokens = 200
	sess.Cost = 1.50
	sess, err = sessions.Save(t.Context(), sess)
	require.NoError(t, err)

	return NewUsageTool(sessions, maxSessionCost), sess
}

func runUsage(t *testing.T, tool fantasy.AgentTool, sessionID string) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(UsageParams{})
	require.NoError(t, err)

	res, err := tool.Run(
		context.WithValue(t.Context(), SessionIDContextKey, sessionID),
		fantasy.ToolCall{ID: "c", Name: UsageToolName, Input: string(input)},
	)
	require.NoError(t, err)
	return res
}

func TestUsageToolReportsTokensAndCost(t *testing.T) {
	t.Parallel()
	tool, sess := newUsageFixture(t, 0)

	res := runUsage(t, tool, sess.ID)

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "1000 prompt tokens")
	require.Contains(t, res.Content, "200 completion tokens")
	require.Contains(t, res.Content, "$1.5000")
	require.NotContains(t, res.Content, "Budget", "no budget is configured")
}

func TestUsageToolReportsRemainingBudget(t *testing.T) {
	t.Parallel()
	tool, sess := newUsageFixture(t, 5.00)

	res := runUsage(t, tool, sess.ID)

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "$3.5000 of $5.0000 remaining")

	var meta UsageResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(res.Metadata), &meta))
	require.InDelta(t, 5.00, meta.MaxSessionCost, 1e-9)
	require.InDelta(t, 3.50, meta.Remaining, 1e-9)
}

func TestUsageToolReportsExhaustedBudget(t *testing.T) {
	t.Parallel()
	tool, sess := newUsageFixture(t, 1.00)

	res := runUsage(t, tool, sess.ID)

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "Budget: exhausted")
}

func TestUsageToolRequiresASession(t *testing.T) {
	t.Parallel()
	tool, _ := newUsageFixture(t, 0)

	input, err := json.Marshal(UsageParams{})
	require.NoError(t, err)

	_, err = tool.Run(t.Context(), fantasy.ToolCall{ID: "c", Name: UsageToolName, Input: string(input)})
	require.Error(t, err)
}
