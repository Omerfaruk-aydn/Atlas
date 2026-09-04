package agent

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/google/uuid"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/prompt"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/csync"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
)

//go:embed templates/vibe_tool.md
var vibeToolDescription string

const VibeToolName = "vibe"

// vibeDoneSentinel is what a worker replies with (and nothing else) to
// signal it judges the goal complete. Checked as a substring, not an
// exact match, since a model does not always follow "reply with exactly
// X" literally.
const vibeDoneSentinel = "VIBE_DONE"

const (
	defaultVibeMaxTurns = 25
	vibeMaxTurnsCeiling = 100
	// vibeTurnTimeout bounds one worker turn. Generous relative to
	// advisorTimeout/escalateTimeout: a worker turn is real, possibly
	// tool-using work toward the goal, not a review.
	vibeTurnTimeout = 5 * time.Minute
)

// VibeStatus is a worker's lifecycle state.
type VibeStatus string

const (
	VibeRunning VibeStatus = "running"
	VibeDone    VibeStatus = "done"
	VibeStopped VibeStatus = "stopped"
	VibeFailed  VibeStatus = "failed"
)

// VibeWorkerInfo is a point-in-time snapshot of a worker, safe to read
// without holding the worker's own lock.
type VibeWorkerInfo struct {
	ID         string
	Goal       string
	AgentName  string
	SessionID  string
	Status     VibeStatus
	Turns      int
	MaxTurns   int
	LastOutput string
	Error      string
}

// vibeWorker is a persistent background worker: it keeps calling its
// agent with fresh prompts (its own last progress, plus any director
// notes queued via the direct action) until it says vibeDoneSentinel,
// runs out of turns, is stopped, or a turn fails outright. All mutable
// fields are behind mu since the loop goroutine and tool-call handlers
// both touch them.
type vibeWorker struct {
	mu sync.Mutex

	id              string
	goal            string
	agentName       string
	parentSessionID string
	maxTurns        int

	sessionID  string
	status     VibeStatus
	turns      int
	lastOutput string
	errMsg     string
	notes      []string

	cancel context.CancelFunc
}

func (w *vibeWorker) info() VibeWorkerInfo {
	w.mu.Lock()
	defer w.mu.Unlock()
	return VibeWorkerInfo{
		ID:         w.id,
		Goal:       w.goal,
		AgentName:  w.agentName,
		SessionID:  w.sessionID,
		Status:     w.status,
		Turns:      w.turns,
		MaxTurns:   w.maxTurns,
		LastOutput: w.lastOutput,
		Error:      w.errMsg,
	}
}

func (w *vibeWorker) addNote(note string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.notes = append(w.notes, note)
}

// drainNotes returns and clears every note queued since the last drain,
// so each note is folded into exactly one upcoming turn's prompt.
func (w *vibeWorker) drainNotes() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	notes := w.notes
	w.notes = nil
	return notes
}

func (w *vibeWorker) setSessionID(id string) {
	w.mu.Lock()
	w.sessionID = id
	w.mu.Unlock()
}

func (w *vibeWorker) setStatus(s VibeStatus) {
	w.mu.Lock()
	w.status = s
	w.mu.Unlock()
}

func (w *vibeWorker) fail(msg string) {
	w.mu.Lock()
	w.status = VibeFailed
	w.errMsg = msg
	w.mu.Unlock()
}

// recordTurn bumps the turn counter, stores the turn's output, and
// reports whether the worker has now reached its turn ceiling.
func (w *vibeWorker) recordTurn(output string) (atLimit bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.turns++
	w.lastOutput = output
	return w.turns >= w.maxTurns
}

// stop cancels the worker's context. The running turn (if any) finishes
// on its own timeout or completion; no further turn starts.
func (w *vibeWorker) stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// startVibeWorker creates the worker's session and launches its loop in
// a detached goroutine, returning immediately -- the caller (a tool
// call) does not wait for the worker to make any progress, let alone
// finish.
func (c *coordinator) startVibeWorker(parentSessionID string, runAgent SessionAgent, agentName, goal string, maxTurns int) *vibeWorker {
	ctx, cancel := context.WithCancel(context.Background())
	w := &vibeWorker{
		id:              uuid.New().String(),
		goal:            goal,
		agentName:       agentName,
		parentSessionID: parentSessionID,
		maxTurns:        maxTurns,
		status:          VibeRunning,
		cancel:          cancel,
	}

	go c.runVibeLoop(ctx, w, runAgent)
	return w
}

func (c *coordinator) runVibeLoop(ctx context.Context, w *vibeWorker, runAgent SessionAgent) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Vibe worker panicked", "id", w.id, "panic", r)
			w.fail(fmt.Sprintf("panicked: %v", r))
		}
	}()

	session, err := c.sessions.CreateTaskSession(ctx, "vibe-"+w.id, w.parentSessionID, "Vibe: "+w.goal)
	if err != nil {
		w.fail(fmt.Sprintf("create session: %s", err))
		return
	}
	w.setSessionID(session.ID)
	if c.teams != nil {
		c.teams.Join(w.parentSessionID, session.ID)
	}

	model := runAgent.Model()
	maxTokens := model.CatwalkCfg.DefaultMaxTokens
	if model.ModelCfg.MaxTokens != 0 {
		maxTokens = model.ModelCfg.MaxTokens
	}
	providerCfg, ok := c.cfg.Config().Providers.Get(model.ModelCfg.Provider)
	if !ok {
		w.fail(errModelProviderNotConfigured.Error())
		return
	}

	prompt := w.goal
	for {
		select {
		case <-ctx.Done():
			w.setStatus(VibeStopped)
			return
		default:
		}

		if notes := w.drainNotes(); len(notes) > 0 {
			prompt = "Director guidance:\n- " + strings.Join(notes, "\n- ") + "\n\n" + prompt
		}

		turnCtx, cancelTurn := context.WithTimeout(ctx, vibeTurnTimeout)
		result, err := runAgent.Run(turnCtx, SessionAgentCall{
			SessionID:        session.ID,
			Prompt:           prompt,
			MaxOutputTokens:  maxTokens,
			ProviderOptions:  getProviderOptions(model, providerCfg),
			Temperature:      model.ModelCfg.Temperature,
			TopP:             model.ModelCfg.TopP,
			TopK:             model.ModelCfg.TopK,
			FrequencyPenalty: model.ModelCfg.FrequencyPenalty,
			PresencePenalty:  model.ModelCfg.PresencePenalty,
			NonInteractive:   true,
		})
		cancelTurn()

		if err != nil {
			w.fail(err.Error())
			return
		}

		if updateErr := c.updateParentSessionCost(ctx, session.ID, w.parentSessionID); updateErr != nil {
			slog.Warn("Failed to update parent session cost for vibe worker", "id", w.id, "error", updateErr)
		}

		output := subAgentOutput(result)
		atLimit := w.recordTurn(output)

		if strings.Contains(output, vibeDoneSentinel) {
			w.setStatus(VibeDone)
			return
		}
		if atLimit {
			w.setStatus(VibeStopped)
			return
		}

		prompt = "Your previous progress:\n" + truncateForAdvisor(output) +
			"\n\nContinue working toward the goal. Once it is fully done, reply with exactly \"" + vibeDoneSentinel + "\" and nothing else."
	}
}

type VibeParams struct {
	Action string `json:"action" description:"start, direct, status, list, or stop"`
	ID     string `json:"id,omitempty" description:"Worker ID. Required for direct, status, and stop."`
	Goal   string `json:"goal,omitempty" description:"For start: what the worker should keep working toward."`
	// AgentName optionally names a configured subagent to run the
	// worker on, instead of the default agent.
	AgentName string `json:"agent_name,omitempty" description:"For start: a configured subagent to run the worker on, instead of the default agent."`
	MaxTurns  int    `json:"max_turns,omitempty" description:"For start: safety cap on turns before the worker stops on its own. Default 25, hard ceiling 100."`
	Note      string `json:"note,omitempty" description:"For direct: new guidance for the worker's next turn."`
}

type VibeResponseMetadata struct {
	ID     string `json:"id,omitempty"`
	Status string `json:"status,omitempty"`
}

func (c *coordinator) vibeTool(ctx context.Context) (fantasy.AgentTool, error) {
	agentCfg, ok := c.cfg.Config().Agents[config.AgentTask]
	if !ok {
		return nil, errors.New("task agent not configured")
	}
	taskPromptTemplate, err := taskPrompt(prompt.WithWorkingDir(c.cfg.WorkingDir()))
	if err != nil {
		return nil, err
	}
	defaultAgent, err := c.buildAgent(ctx, taskPromptTemplate, agentCfg, true)
	if err != nil {
		return nil, err
	}

	var opts *config.Options
	if opts = c.cfg.Config().Options; opts == nil {
		opts = &config.Options{}
	}
	discovered := subagents.Discover(opts.SubagentsPaths)
	subagentInstances := csync.NewMap[string, SessionAgent]()

	// Workers started from this tool build (one per session agent) --
	// not persisted anywhere else, so a worker is only visible to
	// direct/status/list/stop calls within the session that started it.
	workers := csync.NewMap[string, *vibeWorker]()

	return fantasy.NewParallelAgentTool(
		VibeToolName,
		vibeToolDescription,
		func(ctx context.Context, params VibeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := tools.GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session id missing from context")
			}

			switch strings.ToLower(strings.TrimSpace(params.Action)) {
			case "start":
				return c.vibeStart(ctx, workers, agentCfg, discovered, subagentInstances, defaultAgent, sessionID, params)
			case "direct":
				return vibeDirect(workers, params)
			case "status":
				return vibeStatusAction(workers, params)
			case "list":
				return vibeList(workers)
			case "stop":
				return vibeStop(workers, params)
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"unknown action %q: use start, direct, status, list, or stop", params.Action)), nil
			}
		},
	), nil
}

func (c *coordinator) vibeStart(
	ctx context.Context,
	workers *csync.Map[string, *vibeWorker],
	agentCfg config.Agent,
	discovered []*subagents.Subagent,
	subagentInstances *csync.Map[string, SessionAgent],
	defaultAgent SessionAgent,
	sessionID string,
	params VibeParams,
) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.Goal) == "" {
		return fantasy.NewTextErrorResponse("goal is required for start"), nil
	}

	runAgent := defaultAgent
	if params.AgentName != "" {
		resolved, err := c.resolveSubagent(ctx, agentCfg, discovered, subagentInstances, params.AgentName)
		if err != nil {
			return fantasy.NewTextErrorResponse(err.Error()), nil
		}
		runAgent = resolved
	}

	maxTurns := params.MaxTurns
	switch {
	case maxTurns <= 0:
		maxTurns = defaultVibeMaxTurns
	case maxTurns > vibeMaxTurnsCeiling:
		maxTurns = vibeMaxTurnsCeiling
	}

	w := c.startVibeWorker(sessionID, runAgent, params.AgentName, params.Goal, maxTurns)
	workers.Set(w.id, w)

	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(fmt.Sprintf(
			"Vibe worker started (id: %s, max_turns: %d). Goal: %s\nCheck progress with action=status, steer it with action=direct, stop it with action=stop.",
			w.id, maxTurns, params.Goal)),
		VibeResponseMetadata{ID: w.id, Status: string(VibeRunning)},
	), nil
}

func vibeDirect(workers *csync.Map[string, *vibeWorker], params VibeParams) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.ID) == "" {
		return fantasy.NewTextErrorResponse("id is required for direct"), nil
	}
	if strings.TrimSpace(params.Note) == "" {
		return fantasy.NewTextErrorResponse("note is required for direct"), nil
	}
	w, ok := workers.Get(params.ID)
	if !ok {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("no vibe worker with id %q", params.ID)), nil
	}
	w.addNote(params.Note)
	return fantasy.NewTextResponse("Noted -- will be included in the worker's next turn."), nil
}

func vibeStatusAction(workers *csync.Map[string, *vibeWorker], params VibeParams) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.ID) == "" {
		return fantasy.NewTextErrorResponse("id is required for status"), nil
	}
	w, ok := workers.Get(params.ID)
	if !ok {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("no vibe worker with id %q", params.ID)), nil
	}
	info := w.info()
	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(formatVibeInfo(info)),
		VibeResponseMetadata{ID: info.ID, Status: string(info.Status)},
	), nil
}

func vibeList(workers *csync.Map[string, *vibeWorker]) (fantasy.ToolResponse, error) {
	var infos []VibeWorkerInfo
	for w := range workers.Seq() {
		infos = append(infos, w.info())
	}
	if len(infos) == 0 {
		return fantasy.NewTextResponse("No vibe workers started in this session."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d vibe worker(s).\n", len(infos))
	for _, info := range infos {
		b.WriteString("\n")
		b.WriteString(formatVibeInfo(info))
		b.WriteString("\n")
	}
	return fantasy.NewTextResponse(b.String()), nil
}

func vibeStop(workers *csync.Map[string, *vibeWorker], params VibeParams) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.ID) == "" {
		return fantasy.NewTextErrorResponse("id is required for stop"), nil
	}
	w, ok := workers.Get(params.ID)
	if !ok {
		return fantasy.NewTextErrorResponse(fmt.Sprintf("no vibe worker with id %q", params.ID)), nil
	}
	w.stop()
	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(fmt.Sprintf("Stopping vibe worker %s -- its current turn (if any) will finish, then it stops.", params.ID)),
		VibeResponseMetadata{ID: params.ID, Status: string(VibeStopped)},
	), nil
}

func formatVibeInfo(info VibeWorkerInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s -- turn %d/%d\n", info.ID, info.Status, info.Turns, info.MaxTurns)
	fmt.Fprintf(&b, "Goal: %s\n", info.Goal)
	if info.AgentName != "" {
		fmt.Fprintf(&b, "Agent: %s\n", info.AgentName)
	}
	if info.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", info.Error)
	}
	if info.LastOutput != "" {
		fmt.Fprintf(&b, "Last output: %s\n", truncateForAdvisor(info.LastOutput))
	}
	return b.String()
}
