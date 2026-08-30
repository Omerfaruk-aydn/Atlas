// Package agent implements the core agentic loop: send history to the
// configured LLM provider, stream the reply, and dispatch any tool calls
// the model requests before looping back for another round. It knows
// nothing about the TUI — callers drain Event values off the channel
// returned by Run, and answer approval requests via Approve.
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/omerfarukaydin/atlas/internal/llm"
	"github.com/omerfarukaydin/atlas/internal/tools"
)

const (
	defaultMaxTokens = 4096
	// maxToolRounds guards against a runaway tool-call loop (e.g. a model
	// stuck repeatedly calling a tool) from running forever.
	maxToolRounds = 25
)

type Agent struct {
	provider    llm.Provider
	system      string
	history     History
	registry    *tools.Registry
	autoApprove bool

	approvalMu      sync.Mutex
	pendingApproval chan bool

	// ambientDelay/Interval mirror the package consts of the same purpose
	// but are per-instance so tests can shrink them instead of waiting on
	// the real 8s/10s timings.
	ambientDelay    time.Duration
	ambientInterval time.Duration
	ambientTick     time.Duration
}

func New(provider llm.Provider, system string, registry *tools.Registry, autoApprove bool) *Agent {
	return &Agent{
		provider: provider, system: system, registry: registry, autoApprove: autoApprove,
		ambientDelay: ambientCharmDelay, ambientInterval: ambientCharmInterval, ambientTick: time.Second,
	}
}

func (a *Agent) SetProvider(p llm.Provider) { a.provider = p }

// ToolNames lists the names of every tool registered for this session, for
// display purposes (e.g. a session-info panel). Empty if no registry is
// configured.
func (a *Agent) ToolNames() []string {
	if a.registry == nil {
		return nil
	}
	defs := a.registry.ToolDefs()
	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	return names
}

func (a *Agent) ProviderName() string { return a.provider.Name() }

// CurrentModel returns the model the active provider will use next.
func (a *Agent) CurrentModel() string { return a.provider.Model() }

// SetModel switches the active provider's model without changing provider.
func (a *Agent) SetModel(model string) { a.provider.SetModel(model) }

// ListModels queries the active provider for models available to its API key.
func (a *Agent) ListModels(ctx context.Context) ([]string, error) {
	return a.provider.ListModels(ctx)
}

// Approve answers the most recent EventApprovalRequest. It is a no-op if no
// approval is currently pending.
func (a *Agent) Approve(approved bool) {
	a.approvalMu.Lock()
	ch := a.pendingApproval
	a.pendingApproval = nil
	a.approvalMu.Unlock()
	if ch != nil {
		ch <- approved
	}
}

// Run starts one turn: appends userMsg to history, streams the provider's
// reply, executes any requested tool calls (looping back to the model with
// their results), and emits Events throughout. The returned channel is
// closed when the turn fully ends (EventTurnDone or EventError sent).
func (a *Agent) Run(ctx context.Context, userMsg string) <-chan Event {
	a.history.AppendUser(userMsg)
	events := make(chan Event)
	go a.runTurn(ctx, events)
	return events
}

func (a *Agent) runTurn(ctx context.Context, out chan<- Event) {
	defer close(out)

	for round := 0; ; round++ {
		if round >= maxToolRounds {
			emit(ctx, out, Event{Type: EventError, Err: fmt.Errorf("çok fazla araç turu (%d), durduruldu", maxToolRounds)})
			return
		}

		var toolDefs []llm.ToolDef
		if a.registry != nil {
			toolDefs = a.registry.ToolDefs()
		}

		req := llm.Request{
			System:    a.system,
			Messages:  a.history.Messages(),
			Tools:     toolDefs,
			MaxTokens: defaultMaxTokens,
		}

		stream, err := a.provider.StreamChat(ctx, req)
		if err != nil {
			emit(ctx, out, Event{Type: EventError, Err: err})
			return
		}

		assistantText, calls, ok := a.consumeStream(ctx, out, stream)
		if !ok {
			return
		}

		var blocks []llm.ContentBlock
		if assistantText != "" {
			blocks = append(blocks, llm.ContentBlock{Type: "text", Text: assistantText})
		}
		blocks = append(blocks, calls...)
		if len(blocks) > 0 {
			a.history.Append(llm.Message{Role: llm.RoleAssistant, Content: blocks})
		}

		if len(calls) == 0 {
			emit(ctx, out, Event{Type: EventTurnDone})
			return
		}

		results, ok := a.executeToolCalls(ctx, out, calls)
		if !ok {
			return
		}
		a.history.Append(llm.Message{Role: llm.RoleUser, Content: results})
		// Loop back: send the updated history (including tool results) to
		// the model for its next response.
	}
}

// consumeStream drains one provider.StreamChat channel, forwarding text and
// usage events, and collecting any tool_use blocks the model requested.
// Returns ok=false if the context was canceled mid-stream.
func (a *Agent) consumeStream(ctx context.Context, out chan<- Event, stream <-chan llm.StreamEvent) (string, []llm.ContentBlock, bool) {
	var assistantText string
	var calls []llm.ContentBlock

	for {
		var ev llm.StreamEvent
		var open bool
		select {
		case ev, open = <-stream:
			if !open {
				return assistantText, calls, true
			}
		case <-ctx.Done():
			return "", nil, false
		}

		switch ev.Type {
		case llm.EventTextDelta:
			assistantText += ev.TextDelta
			if !emit(ctx, out, Event{Type: EventTextDelta, TextDelta: ev.TextDelta}) {
				return "", nil, false
			}
		case llm.EventToolCall:
			calls = append(calls, llm.ContentBlock{
				Type:      "tool_use",
				ToolUseID: ev.ToolCallID,
				ToolName:  ev.ToolName,
				ToolInput: ev.ToolInput,
			})
		case llm.EventUsage:
			if !emit(ctx, out, Event{Type: EventUsage, InputTok: ev.Usage.InputTokens}) {
				return "", nil, false
			}
		case llm.EventTurnEnd:
			if !emit(ctx, out, Event{Type: EventUsage, OutputTok: ev.Usage.OutputTokens}) {
				return "", nil, false
			}
		case llm.EventError:
			emit(ctx, out, Event{Type: EventError, Err: ev.Err})
			return "", nil, false
		}
	}
}

// executeToolCalls runs each requested tool sequentially (gating on user
// approval where required) and returns the resulting tool_result blocks in
// the same order, ready to be appended to history as one user-role message.
func (a *Agent) executeToolCalls(ctx context.Context, out chan<- Event, calls []llm.ContentBlock) ([]llm.ContentBlock, bool) {
	results := make([]llm.ContentBlock, 0, len(calls))
	for _, call := range calls {
		result := a.executeOneTool(ctx, out, call)
		if result == nil {
			return nil, false
		}
		results = append(results, *result)
	}
	return results, true
}

func (a *Agent) executeOneTool(ctx context.Context, out chan<- Event, call llm.ContentBlock) *llm.ContentBlock {
	mkResult := func(content string, isErr bool) *llm.ContentBlock {
		return &llm.ContentBlock{
			Type:              "tool_result",
			ToolResultID:      call.ToolUseID,
			ToolName:          call.ToolName,
			ToolResultContent: content,
			ToolResultIsError: isErr,
		}
	}

	if a.registry == nil {
		return mkResult("araç sistemi bu oturumda etkin değil", true)
	}
	tool, ok := a.registry.Get(call.ToolName)
	if !ok {
		return mkResult("bilinmeyen araç: "+call.ToolName, true)
	}

	if tool.RequiresApproval() && !a.autoApprove {
		// pendingApproval must be set before the request is emitted: emit
		// blocks until the TUI receives it, and the TUI may call Approve
		// synchronously right after — if that races ahead of setting the
		// channel here, the approval is silently dropped and this goroutine
		// blocks forever waiting on a channel nobody can reach.
		approvalCh := make(chan bool, 1)
		a.approvalMu.Lock()
		a.pendingApproval = approvalCh
		a.approvalMu.Unlock()

		approvalEvent := Event{
			Type:       EventApprovalRequest,
			ToolCallID: call.ToolUseID,
			ToolName:   call.ToolName,
			ToolInput:  call.ToolInput,
		}
		if previewer, ok := tool.(tools.Previewable); ok {
			if p, err := previewer.Preview(call.ToolInput); err == nil {
				approvalEvent.PreviewPath, approvalEvent.PreviewOld, approvalEvent.PreviewNew = p.Path, p.Old, p.New
			}
		}

		if !emit(ctx, out, approvalEvent) {
			return nil
		}

		var approved bool
		select {
		case approved = <-approvalCh:
		case <-ctx.Done():
			return nil
		}

		if !approved {
			res := mkResult("kullanıcı bu aracı çalıştırmayı reddetti", true)
			if !emit(ctx, out, Event{
				Type: EventToolResult, ToolCallID: call.ToolUseID, ToolName: call.ToolName,
				ToolOutput: res.ToolResultContent, ToolIsError: true,
			}) {
				return nil
			}
			return res
		}
	}

	if !emit(ctx, out, Event{Type: EventToolStart, ToolCallID: call.ToolUseID, ToolName: call.ToolName, ToolInput: call.ToolInput}) {
		return nil
	}

	content, isErr, ok := a.runToolWithAmbientCharms(ctx, out, call, tool)
	if !ok {
		return nil
	}

	if !emit(ctx, out, Event{
		Type: EventToolResult, ToolCallID: call.ToolUseID, ToolName: call.ToolName,
		ToolOutput: content, ToolIsError: isErr,
	}) {
		return nil
	}

	return mkResult(content, isErr)
}

const (
	// ambientCharmDelay is how long a tool must run before we consider it
	// "unresponsive-feeling" and start injecting ambient status lines.
	ambientCharmDelay = 8 * time.Second
	// ambientCharmInterval is the minimum gap between successive charms
	// for the same call.
	ambientCharmInterval = 10 * time.Second
	// maxAmbientCharmsPerCall caps how many charms one slow call can emit.
	maxAmbientCharmsPerCall = 2
)

// runToolWithAmbientCharms executes tool.Execute in the background while
// watching the clock: if the call is still running after ambientCharmDelay,
// it emits an EventAmbient status line (max maxAmbientCharmsPerCall, spaced
// at least ambientCharmInterval apart) so a slow tool doesn't read as a
// hang. The TUI decides how to phrase/display the ambient line; this only
// reports elapsed time.
func (a *Agent) runToolWithAmbientCharms(ctx context.Context, out chan<- Event, call llm.ContentBlock, tool tools.Tool) (content string, isErr bool, ok bool) {
	type outcome struct {
		result tools.Result
		err    error
	}
	resultCh := make(chan outcome, 1)
	go func() {
		r, err := tool.Execute(ctx, call.ToolInput)
		resultCh <- outcome{r, err}
	}()

	start := time.Now()
	ticker := time.NewTicker(a.ambientTick)
	defer ticker.Stop()
	charmCount := 0
	var lastCharm time.Time

	for {
		select {
		case o := <-resultCh:
			content, isErr = o.result.Content, o.result.IsError
			if o.err != nil {
				content, isErr = o.err.Error(), true
			}
			return content, isErr, true

		case <-ticker.C:
			elapsed := time.Since(start)
			if elapsed < a.ambientDelay || charmCount >= maxAmbientCharmsPerCall {
				continue
			}
			if !lastCharm.IsZero() && time.Since(lastCharm) < a.ambientInterval {
				continue
			}
			if !emit(ctx, out, Event{
				Type: EventAmbient, ToolCallID: call.ToolUseID, ToolName: call.ToolName,
				ElapsedMS: elapsed.Milliseconds(),
			}) {
				return "", false, false
			}
			charmCount++
			lastCharm = time.Now()

		case <-ctx.Done():
			return "", false, false
		}
	}
}

// emit sends ev on out unless ctx is done first. Returns false if the
// caller should stop (context canceled).
func emit(ctx context.Context, out chan<- Event, ev Event) bool {
	select {
	case out <- ev:
		return true
	case <-ctx.Done():
		return false
	}
}
