package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/omerfarukaydin/atlas/internal/llm"
	"github.com/omerfarukaydin/atlas/internal/tools"
)

// fakeProvider emits a scripted sequence of StreamEvents, simulating a
// real provider's SSE stream without touching the network.
type fakeProvider struct {
	events    []llm.StreamEvent
	lastReq   llm.Request
	callCount int
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Model() string         { return "fake-model" }
func (f *fakeProvider) SetModel(model string) {}
func (f *fakeProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{"fake-model"}, nil
}

func (f *fakeProvider) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	f.callCount++
	f.lastReq = req
	out := make(chan llm.StreamEvent, len(f.events))
	for _, e := range f.events {
		out <- e
	}
	close(out)
	return out, nil
}

func collect(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var got []Event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return got
			}
			got = append(got, ev)
		case <-timeout:
			t.Fatal("timed out waiting for agent events")
		}
	}
}

func TestAgentRunStreamsTextAndEndsTurn(t *testing.T) {
	fp := &fakeProvider{events: []llm.StreamEvent{
		{Type: llm.EventTextDelta, TextDelta: "Merhaba"},
		{Type: llm.EventTextDelta, TextDelta: ", dünya"},
		{Type: llm.EventUsage, Usage: llm.Usage{InputTokens: 12}},
		{Type: llm.EventTurnEnd, StopReason: "end_turn", Usage: llm.Usage{OutputTokens: 5}},
	}}

	ag := New(fp, "system prompt", nil, false)
	ch := ag.Run(context.Background(), "selam")
	events := collect(t, ch)

	var text string
	var sawDone bool
	var inTok, outTok int64
	for _, ev := range events {
		switch ev.Type {
		case EventTextDelta:
			text += ev.TextDelta
		case EventUsage:
			inTok += ev.InputTok
			outTok += ev.OutputTok
		case EventTurnDone:
			sawDone = true
		case EventError:
			t.Fatalf("unexpected error event: %v", ev.Err)
		}
	}

	if text != "Merhaba, dünya" {
		t.Errorf("expected accumulated text %q, got %q", "Merhaba, dünya", text)
	}
	if !sawDone {
		t.Error("expected a turn_done event")
	}
	if inTok != 12 || outTok != 5 {
		t.Errorf("expected usage 12in/5out, got %din/%dout", inTok, outTok)
	}

	if len(ag.history.Messages()) != 2 {
		t.Fatalf("expected 2 messages in history (user+assistant), got %d", len(ag.history.Messages()))
	}
	if ag.history.Messages()[1].Content[0].Text != "Merhaba, dünya" {
		t.Errorf("assistant reply not appended to history correctly")
	}

	if fp.lastReq.System != "system prompt" {
		t.Errorf("expected system prompt to be forwarded, got %q", fp.lastReq.System)
	}
}

func TestAgentRunSurfacesProviderError(t *testing.T) {
	fp := &fakeProvider{events: []llm.StreamEvent{
		{Type: llm.EventError, Err: errors.New("boom")},
	}}

	ag := New(fp, "", nil, false)
	ch := ag.Run(context.Background(), "selam")
	events := collect(t, ch)

	if len(events) != 1 || events[0].Type != EventError {
		t.Fatalf("expected single error event, got %+v", events)
	}
	if events[0].Err.Error() != "boom" {
		t.Errorf("expected error 'boom', got %v", events[0].Err)
	}

	// A failed turn should not pollute history with an empty assistant reply.
	if len(ag.history.Messages()) != 1 {
		t.Errorf("expected only the user message in history after an error, got %d", len(ag.history.Messages()))
	}
}

func TestAgentRunCancelsOnContextDone(t *testing.T) {
	// A provider that never closes its channel; the agent must stop
	// forwarding events once the context is canceled instead of hanging.
	block := make(chan llm.StreamEvent)
	fp := &blockingProvider{ch: block}

	ag := New(fp, "", nil, false)
	ctx, cancel := context.WithCancel(context.Background())
	ch := ag.Run(ctx, "selam")
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to close without further events after cancel")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not stop after context cancellation")
	}
}

type blockingProvider struct{ ch chan llm.StreamEvent }

func (b *blockingProvider) Name() string                                     { return "blocking" }
func (b *blockingProvider) Model() string                                    { return "" }
func (b *blockingProvider) SetModel(model string)                            {}
func (b *blockingProvider) ListModels(ctx context.Context) ([]string, error) { return nil, nil }
func (b *blockingProvider) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	return b.ch, nil
}

// scriptedProvider returns a different pre-scripted event sequence on each
// successive StreamChat call, letting tests simulate a tool-call round
// followed by the model's next response once it sees the tool's result.
type scriptedProvider struct {
	rounds [][]llm.StreamEvent
	calls  int
}

func (s *scriptedProvider) Name() string                                     { return "scripted" }
func (s *scriptedProvider) Model() string                                    { return "scripted-model" }
func (s *scriptedProvider) SetModel(model string)                            {}
func (s *scriptedProvider) ListModels(ctx context.Context) ([]string, error) { return nil, nil }
func (s *scriptedProvider) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	idx := s.calls
	s.calls++
	var round []llm.StreamEvent
	if idx < len(s.rounds) {
		round = s.rounds[idx]
	}
	out := make(chan llm.StreamEvent, len(round))
	for _, e := range round {
		out <- e
	}
	close(out)
	return out, nil
}

// fakeTool is a minimal tools.Tool for exercising the agent's tool-dispatch
// and approval-gating logic without touching the filesystem or a shell.
type fakeTool struct {
	name             string
	requiresApproval bool
	resultText       string
	called           int
}

func (t *fakeTool) Name() string                 { return t.name }
func (t *fakeTool) Description() string          { return "fake tool for tests" }
func (t *fakeTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *fakeTool) RequiresApproval() bool       { return t.requiresApproval }
func (t *fakeTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	t.called++
	return tools.Result{Content: t.resultText}, nil
}

func TestAgentExecutesToolWithoutApproval(t *testing.T) {
	tool := &fakeTool{name: "no_approval_tool", resultText: "42"}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "no_approval_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "Cevap: 42"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, false)
	ch := ag.Run(context.Background(), "kaç eder?")
	events := collect(t, ch)

	var text string
	var sawToolStart, sawToolResult, sawDone bool
	for _, ev := range events {
		switch ev.Type {
		case EventTextDelta:
			text += ev.TextDelta
		case EventToolStart:
			sawToolStart = true
		case EventToolResult:
			sawToolResult = true
			if ev.ToolOutput != "42" {
				t.Errorf("expected tool output 42, got %q", ev.ToolOutput)
			}
		case EventTurnDone:
			sawDone = true
		case EventApprovalRequest:
			t.Error("did not expect an approval request for a no-approval tool")
		case EventError:
			t.Fatalf("unexpected error: %v", ev.Err)
		}
	}

	if tool.called != 1 {
		t.Errorf("expected tool to be called once, got %d", tool.called)
	}
	if !sawToolStart || !sawToolResult || !sawDone {
		t.Errorf("expected tool_start, tool_result and turn_done events; got start=%v result=%v done=%v", sawToolStart, sawToolResult, sawDone)
	}
	if text != "Cevap: 42" {
		t.Errorf("expected final text %q, got %q", "Cevap: 42", text)
	}
	if sp.calls != 2 {
		t.Errorf("expected provider to be called twice (initial + after tool result), got %d", sp.calls)
	}

	msgs := ag.history.Messages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 history messages (user, assistant tool_use, user tool_result, assistant text), got %d", len(msgs))
	}
}

func TestAgentApprovalGatingApprove(t *testing.T) {
	tool := &fakeTool{name: "risky_tool", requiresApproval: true, resultText: "done"}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "risky_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "tamamlandı"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, false)
	ch := ag.Run(context.Background(), "riskli bir şey yap")

	approved := false
	var text string
	timeout := time.After(2 * time.Second)
drain:
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				break drain
			}
			switch ev.Type {
			case EventApprovalRequest:
				if approved {
					t.Fatal("received a second approval request unexpectedly")
				}
				approved = true
				ag.Approve(true)
			case EventTextDelta:
				text += ev.TextDelta
			case EventError:
				t.Fatalf("unexpected error: %v", ev.Err)
			}
		case <-timeout:
			t.Fatal("timed out waiting for agent events")
		}
	}

	if !approved {
		t.Fatal("expected an approval request")
	}
	if tool.called != 1 {
		t.Errorf("expected tool to be called once after approval, got %d", tool.called)
	}
	if text != "tamamlandı" {
		t.Errorf("expected final text %q, got %q", "tamamlandı", text)
	}
}

func TestAgentApprovalGatingReject(t *testing.T) {
	tool := &fakeTool{name: "risky_tool", requiresApproval: true, resultText: "done"}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "risky_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "tamam, yapmadım"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, false)
	ch := ag.Run(context.Background(), "riskli bir şey yap")

	sawApproval := false
	timeout := time.After(2 * time.Second)
drain:
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				break drain
			}
			if ev.Type == EventApprovalRequest {
				sawApproval = true
				ag.Approve(false)
			}
		case <-timeout:
			t.Fatal("timed out waiting for agent events")
		}
	}

	if !sawApproval {
		t.Fatal("expected an approval request")
	}
	if tool.called != 0 {
		t.Errorf("expected tool NOT to be called after rejection, got %d calls", tool.called)
	}
}

// slowTool sleeps for a fixed duration before returning, so tests can
// exercise the ambient-charm mechanism without waiting on the real
// 8s/10s production timings.
type slowTool struct {
	name     string
	duration time.Duration
}

func (t *slowTool) Name() string                 { return t.name }
func (t *slowTool) Description() string          { return "slow tool for tests" }
func (t *slowTool) InputSchema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (t *slowTool) RequiresApproval() bool       { return false }
func (t *slowTool) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	select {
	case <-time.After(t.duration):
	case <-ctx.Done():
	}
	return tools.Result{Content: "done"}, nil
}

func TestAgentEmitsAmbientCharmsForSlowTool(t *testing.T) {
	tool := &slowTool{name: "slow_tool", duration: 120 * time.Millisecond}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "slow_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "bitti"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, false)
	// Shrink the production 8s/10s/1s timings so the tool's 120ms run
	// still crosses the "ambient" threshold within a fast test.
	ag.ambientDelay = 20 * time.Millisecond
	ag.ambientInterval = 30 * time.Millisecond
	ag.ambientTick = 10 * time.Millisecond

	ch := ag.Run(context.Background(), "yavaş bir şey yap")
	events := collect(t, ch)

	var ambientCount int
	for _, ev := range events {
		if ev.Type == EventAmbient {
			ambientCount++
			if ev.ToolName != "slow_tool" {
				t.Errorf("expected ambient event for slow_tool, got %q", ev.ToolName)
			}
			if ev.ElapsedMS <= 0 {
				t.Error("expected a positive ElapsedMS on an ambient event")
			}
		}
	}

	if ambientCount == 0 {
		t.Fatal("expected at least one ambient charm for a tool exceeding ambientDelay")
	}
	if ambientCount > maxAmbientCharmsPerCall {
		t.Errorf("expected at most %d ambient charms, got %d", maxAmbientCharmsPerCall, ambientCount)
	}
}

func TestAgentDoesNotEmitAmbientCharmsForFastTool(t *testing.T) {
	tool := &fakeTool{name: "fast_tool", resultText: "ok"}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "fast_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "tamam"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, false)
	ag.ambientDelay = 5 * time.Second // a fast fake tool returns instantly, well under this
	ag.ambientTick = 10 * time.Millisecond

	ch := ag.Run(context.Background(), "hızlı bir şey yap")
	events := collect(t, ch)

	for _, ev := range events {
		if ev.Type == EventAmbient {
			t.Error("did not expect an ambient charm for an instantly-completing tool")
		}
	}
}

func TestAgentAutoApproveSkipsPrompt(t *testing.T) {
	tool := &fakeTool{name: "risky_tool", requiresApproval: true, resultText: "done"}
	reg := tools.NewRegistry()
	reg.Register(tool)

	sp := &scriptedProvider{rounds: [][]llm.StreamEvent{
		{
			{Type: llm.EventToolCall, ToolCallID: "call_1", ToolName: "risky_tool", ToolInput: json.RawMessage(`{}`)},
			{Type: llm.EventTurnEnd, StopReason: "tool_use"},
		},
		{
			{Type: llm.EventTextDelta, TextDelta: "bitti"},
			{Type: llm.EventTurnEnd, StopReason: "end_turn"},
		},
	}}

	ag := New(sp, "", reg, true)
	ch := ag.Run(context.Background(), "riskli bir şey yap")
	events := collect(t, ch)

	for _, ev := range events {
		if ev.Type == EventApprovalRequest {
			t.Fatal("did not expect an approval request when auto_approve is true")
		}
	}
	if tool.called != 1 {
		t.Errorf("expected tool to run automatically, got %d calls", tool.called)
	}
}
