package agent

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/stretchr/testify/require"
)

func TestVibeWorkerNotesAreDrainedOnce(t *testing.T) {
	w := &vibeWorker{}
	require.Empty(t, w.drainNotes())

	w.addNote("first")
	w.addNote("second")
	require.Equal(t, []string{"first", "second"}, w.drainNotes())
	require.Empty(t, w.drainNotes(), "a second drain must come back empty")
}

func TestVibeWorkerRecordTurnReportsLimit(t *testing.T) {
	w := &vibeWorker{maxTurns: 2}
	require.False(t, w.recordTurn("out 1"))
	require.True(t, w.recordTurn("out 2"))

	info := w.info()
	require.Equal(t, 2, info.Turns)
	require.Equal(t, "out 2", info.LastOutput)
}

func TestVibeWorkerFailSetsStatusAndError(t *testing.T) {
	w := &vibeWorker{status: VibeRunning}
	w.fail("boom")

	info := w.info()
	require.Equal(t, VibeFailed, info.Status)
	require.Equal(t, "boom", info.Error)
}

func TestFormatVibeInfo(t *testing.T) {
	out := formatVibeInfo(VibeWorkerInfo{
		ID: "w1", Goal: "migrate the thing", AgentName: "research",
		Status: VibeRunning, Turns: 3, MaxTurns: 25, LastOutput: "made progress",
	})
	require.Contains(t, out, "[w1] running -- turn 3/25")
	require.Contains(t, out, "migrate the thing")
	require.Contains(t, out, "research")
	require.Contains(t, out, "made progress")
}

func TestVibeDirectUnknownID(t *testing.T) {
	workers := csync.NewMap[string, *vibeWorker]()
	resp, err := vibeDirect(workers, VibeParams{ID: "nope", Note: "x"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, `no vibe worker with id "nope"`)
}

func TestVibeDirectRequiresIDAndNote(t *testing.T) {
	workers := csync.NewMap[string, *vibeWorker]()

	resp, err := vibeDirect(workers, VibeParams{Note: "x"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "id is required")

	resp, err = vibeDirect(workers, VibeParams{ID: "w1"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "note is required")
}

func TestVibeDirectQueuesANote(t *testing.T) {
	w := &vibeWorker{id: "w1"}
	workers := csync.NewMap[string, *vibeWorker]()
	workers.Set("w1", w)

	resp, err := vibeDirect(workers, VibeParams{ID: "w1", Note: "focus on the edge cases"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Noted")
	require.Equal(t, []string{"focus on the edge cases"}, w.drainNotes())
}

func TestVibeStatusActionUnknownID(t *testing.T) {
	workers := csync.NewMap[string, *vibeWorker]()
	resp, err := vibeStatusAction(workers, VibeParams{ID: "nope"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, `no vibe worker with id "nope"`)
}

func TestVibeStatusActionReportsSnapshot(t *testing.T) {
	w := &vibeWorker{id: "w1", goal: "do the thing", status: VibeRunning, maxTurns: 25, turns: 2}
	workers := csync.NewMap[string, *vibeWorker]()
	workers.Set("w1", w)

	resp, err := vibeStatusAction(workers, VibeParams{ID: "w1"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "turn 2/25")

	var meta VibeResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.Equal(t, "w1", meta.ID)
	require.Equal(t, "running", meta.Status)
}

func TestVibeListEmpty(t *testing.T) {
	resp, err := vibeList(csync.NewMap[string, *vibeWorker]())
	require.NoError(t, err)
	require.Contains(t, resp.Content, "No vibe workers")
}

func TestVibeListReportsEveryWorker(t *testing.T) {
	workers := csync.NewMap[string, *vibeWorker]()
	workers.Set("w1", &vibeWorker{id: "w1", goal: "goal one", status: VibeRunning, maxTurns: 25})
	workers.Set("w2", &vibeWorker{id: "w2", goal: "goal two", status: VibeDone, maxTurns: 10})

	resp, err := vibeList(workers)
	require.NoError(t, err)
	require.Contains(t, resp.Content, "2 vibe worker(s)")
	require.Contains(t, resp.Content, "goal one")
	require.Contains(t, resp.Content, "goal two")
}

func TestVibeStopUnknownID(t *testing.T) {
	workers := csync.NewMap[string, *vibeWorker]()
	resp, err := vibeStop(workers, VibeParams{ID: "nope"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, `no vibe worker with id "nope"`)
}

func TestVibeStopCancelsTheWorkersContext(t *testing.T) {
	var cancelled bool
	w := &vibeWorker{id: "w1", cancel: func() { cancelled = true }}
	workers := csync.NewMap[string, *vibeWorker]()
	workers.Set("w1", w)

	resp, err := vibeStop(workers, VibeParams{ID: "w1"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "Stopping vibe worker w1")
	require.True(t, cancelled)
}

// TestStartVibeWorkerRunsUntilDone drives the real background loop
// (startVibeWorker/runVibeLoop) with a mock SessionAgent that reports
// done on its first turn -- no network call, since mockSessionAgent.Run
// is a plain function, not a real provider round trip.
func TestStartVibeWorkerRunsUntilDone(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})
	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		require.Equal(t, "do the thing", call.Prompt)
		return agentResultWithText("all set. VIBE_DONE"), nil
	})

	w := coord.startVibeWorker(parentSession.ID, mock, "", "do the thing", 5)
	require.Eventually(t, func() bool {
		return w.info().Status == VibeDone
	}, 2*time.Second, 10*time.Millisecond)

	info := w.info()
	require.Equal(t, 1, info.Turns)
	require.NotEmpty(t, info.SessionID)
}

// TestStartVibeWorkerStopsAtMaxTurns verifies a worker that never says
// VIBE_DONE stops on its own once it hits its turn ceiling, rather than
// running forever.
func TestStartVibeWorkerStopsAtMaxTurns(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})
	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	var calls atomic.Int32
	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		calls.Add(1)
		return agentResultWithText("still working"), nil
	})

	w := coord.startVibeWorker(parentSession.ID, mock, "", "keep going forever", 2)
	require.Eventually(t, func() bool {
		return w.info().Status == VibeStopped
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, int32(2), calls.Load(), "must stop exactly at max_turns, not before or after")
	require.Equal(t, 2, w.info().Turns)
}

// TestStartVibeWorkerFoldsInDirectorNotes verifies a note queued via
// addNote between two turns is folded into the *next* turn's prompt,
// not the one already in flight -- the first turn is held open on a
// channel until the test has queued the note, so the ordering is
// deterministic rather than a race.
func TestStartVibeWorkerFoldsInDirectorNotes(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})
	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	prompts := make(chan string, 2)
	proceed := make(chan struct{})
	var calls atomic.Int32
	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		n := calls.Add(1)
		prompts <- call.Prompt
		if n == 1 {
			<-proceed
			return agentResultWithText("still working"), nil
		}
		return agentResultWithText("VIBE_DONE"), nil
	})

	w := coord.startVibeWorker(parentSession.ID, mock, "", "do the thing", 5)

	first := <-prompts
	require.Equal(t, "do the thing", first)

	w.addNote("focus on the edge cases")
	close(proceed)

	second := <-prompts
	require.Contains(t, second, "focus on the edge cases")

	require.Eventually(t, func() bool {
		return w.info().Status == VibeDone
	}, 2*time.Second, 10*time.Millisecond)
}

// TestStartVibeWorkerStopStopsTheLoop verifies stop() prevents a new
// turn from starting once the in-flight one finishes, even though the
// worker was configured with plenty of turns left.
func TestStartVibeWorkerStopStopsTheLoop(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})
	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	started := make(chan struct{})
	proceed := make(chan struct{})
	var calls atomic.Int32
	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		calls.Add(1)
		close(started)
		<-proceed
		return agentResultWithText("still working"), nil
	})

	w := coord.startVibeWorker(parentSession.ID, mock, "", "do the thing", 50)
	<-started
	w.stop()
	close(proceed)

	require.Eventually(t, func() bool {
		return w.info().Status == VibeStopped
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, int32(1), calls.Load(), "stop must prevent a second turn from starting")
}

func TestVibeStartRequiresGoal(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	resp, err := coord.vibeStart(t.Context(), csync.NewMap[string, *vibeWorker](),
		coord.cfg.Config().Agents[config.AgentTask], nil, csync.NewMap[string, SessionAgent](), nil,
		"session-1", VibeParams{})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "goal is required")
}

func TestVibeStartWithUnknownAgentNameErrors(t *testing.T) {
	coord := hermeticSubagentCoordinator(t)
	coord.credentials = credentials.New()

	resp, err := coord.vibeStart(t.Context(), csync.NewMap[string, *vibeWorker](),
		coord.cfg.Config().Agents[config.AgentTask], nil, csync.NewMap[string, SessionAgent](), nil,
		"session-1", VibeParams{Goal: "do it", AgentName: "nope"})
	require.NoError(t, err)
	require.Contains(t, resp.Content, `no subagent named "nope"`)
}

func TestVibeStartResolvesNamedAgentAndClampsMaxTurns(t *testing.T) {
	const providerID = "test-provider"
	env := testEnv(t)
	coord := newTestCoordinator(t, env, providerID, config.ProviderConfig{ID: providerID})
	parentSession, err := env.sessions.Create(t.Context(), "Parent")
	require.NoError(t, err)

	mock := newMockAgent(providerID, 4096, func(_ context.Context, call SessionAgentCall) (*fantasy.AgentResult, error) {
		return agentResultWithText("VIBE_DONE"), nil
	})
	cache := csync.NewMap[string, SessionAgent]()
	cache.Set("research", mock)

	ctx := context.WithValue(t.Context(), tools.SessionIDContextKey, parentSession.ID)
	resp, err := coord.vibeStart(ctx, csync.NewMap[string, *vibeWorker](),
		config.Agent{}, nil, cache, nil,
		parentSession.ID, VibeParams{Goal: "do it", AgentName: "research", MaxTurns: 9999})
	require.NoError(t, err)
	require.Contains(t, resp.Content, "max_turns: 100")

	var meta VibeResponseMetadata
	require.NoError(t, json.Unmarshal([]byte(resp.Metadata), &meta))
	require.NotEmpty(t, meta.ID)
	require.Equal(t, "running", meta.Status)
}
