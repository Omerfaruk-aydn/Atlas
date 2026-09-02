package agent

import (
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	"github.com/stretchr/testify/require"
)

// newBudgetedAgent builds a sessionAgent with MaxSessionCost set and a
// minimal working model (finishStreamModel, from dispatch_cancel_test.go),
// so a run that is not refused for cost can still complete instead of
// panicking on a nil provider.
func newBudgetedAgent(env fakeEnv, maxCost float64) SessionAgent {
	model := &finishStreamModel{text: "done"}
	return NewSessionAgent(SessionAgentOptions{
		LargeModel:     Model{Model: model, CatwalkCfg: catwalk.Model{ContextWindow: 200000, DefaultMaxTokens: 10000}},
		SmallModel:     Model{Model: model, CatwalkCfg: catwalk.Model{ContextWindow: 200000, DefaultMaxTokens: 10000}},
		IsYolo:         true,
		Sessions:       env.sessions,
		Messages:       env.messages,
		MaxSessionCost: maxCost,
	})
}

func TestRunRefusesOverBudgetSession(t *testing.T) {
	env := testEnv(t)
	agent := newBudgetedAgent(env, 5.00)

	sess, err := env.sessions.Create(t.Context(), "expensive session")
	require.NoError(t, err)
	sess.Cost = 5.00
	_, err = env.sessions.Save(t.Context(), sess)
	require.NoError(t, err)

	_, err = agent.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "one more thing"})

	require.ErrorIs(t, err, ErrSessionBudgetExceeded)
}

func TestRunAllowsSessionUnderBudget(t *testing.T) {
	env := testEnv(t)
	agent := newBudgetedAgent(env, 5.00)

	sess, err := env.sessions.Create(t.Context(), "cheap session")
	require.NoError(t, err)
	sess.Cost = 1.00
	_, err = env.sessions.Save(t.Context(), sess)
	require.NoError(t, err)

	_, err = agent.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "go on"})

	require.NoError(t, err)
}

func TestRunIgnoresBudgetWhenUnset(t *testing.T) {
	env := testEnv(t)
	agent := newBudgetedAgent(env, 0)

	sess, err := env.sessions.Create(t.Context(), "unbounded session")
	require.NoError(t, err)
	sess.Cost = 1_000_000
	_, err = env.sessions.Save(t.Context(), sess)
	require.NoError(t, err)

	_, err = agent.Run(t.Context(), SessionAgentCall{SessionID: sess.ID, Prompt: "go on"})

	require.NoError(t, err, "zero means unbounded, however much has been spent")
}
