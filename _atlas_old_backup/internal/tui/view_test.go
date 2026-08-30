package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/omerfarukaydin/atlas/internal/agent"
	"github.com/omerfarukaydin/atlas/internal/llm"
	"github.com/omerfarukaydin/atlas/internal/tools"
)

// fakeProvider is a minimal llm.Provider for exercising the TUI's render
// pipeline without any real agent/network behavior.
type fakeProvider struct{ model string }

func (f *fakeProvider) Name() string          { return "fake" }
func (f *fakeProvider) Model() string         { return f.model }
func (f *fakeProvider) SetModel(model string) { f.model = model }
func (f *fakeProvider) ListModels(ctx context.Context) ([]string, error) {
	return []string{f.model}, nil
}
func (f *fakeProvider) StreamChat(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

func newTestApp(width, height int) *App {
	ag := agent.New(&fakeProvider{model: "fake-model"}, "", nil, false)
	app := New(ag, nil)
	app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return app
}

func TestViewRendersWithoutPanicAtVariousSizes(t *testing.T) {
	sizes := [][2]int{{80, 24}, {40, 15}, {120, 40}, {20, 8}}
	for _, sz := range sizes {
		app := newTestApp(sz[0], sz[1])
		out := app.View()
		if out == "" {
			t.Errorf("View() returned empty output at %dx%d", sz[0], sz[1])
		}
		if !strings.Contains(out, "Atlas") {
			t.Errorf("expected header to mention Atlas at %dx%d, got:\n%s", sz[0], sz[1], out)
		}
	}
}

func TestViewShowsChatMessagesAndStatus(t *testing.T) {
	app := newTestApp(80, 24)
	app.messages = append(app.messages,
		chatMessage{role: "user", text: "merhaba"},
		chatMessage{role: "assistant", text: "selam"},
	)
	app.render()
	out := app.View()

	if !strings.Contains(out, "Sen") {
		t.Error("expected the user label 'Sen' in the view")
	}
	if !strings.Contains(out, "fake/fake-model") {
		t.Errorf("expected status bar to show provider/model, got:\n%s", out)
	}
	if !strings.Contains(out, "hazır") {
		t.Error("expected idle status indicator when not streaming")
	}
}

func TestViewShowsApprovalPromptWhenPending(t *testing.T) {
	app := newTestApp(80, 24)
	app.pendingApproval = &approvalRequest{toolName: "run_shell", input: []byte(`{"command":"ls"}`)}
	out := app.View()

	if !strings.Contains(out, "run_shell") {
		t.Error("expected the pending tool name to appear in the approval prompt")
	}
	if !strings.Contains(out, "onayla") {
		t.Error("expected the approve/reject hint in the approval prompt")
	}
}

func TestViewShowsDiffPreviewWhenAvailable(t *testing.T) {
	app := newTestApp(80, 24)
	app.pendingApproval = &approvalRequest{
		toolName:    "write_file",
		previewPath: "foo.txt",
		previewOld:  "old",
		previewNew:  "new",
	}
	out := app.View()

	if !strings.Contains(out, "foo.txt") {
		t.Error("expected the preview path to appear in the approval prompt")
	}
}

func TestStatusBarDropsSegmentsOnNarrowWidth(t *testing.T) {
	wide := newTestApp(200, 24)
	wide.lastTurnMS = 1500
	wideOut := wide.renderStatusBar()
	if !strings.Contains(wideOut, "tokens") {
		t.Error("expected tokens segment on a wide status bar")
	}
	if !strings.Contains(wideOut, "1.5s") {
		t.Error("expected duration segment on a wide status bar")
	}

	narrow := newTestApp(35, 24)
	narrow.lastTurnMS = 1500
	narrowOut := narrow.renderStatusBar()
	if lipgloss.Width(narrowOut) > 35 {
		t.Errorf("status bar must not exceed terminal width, got width %d for %q", lipgloss.Width(narrowOut), narrowOut)
	}
	// The pinned left side (status + provider/model) must always survive
	// even when every optional segment has to drop.
	if !strings.Contains(narrowOut, "fake/fake-model") {
		t.Errorf("expected pinned provider/model to survive on a narrow bar, got: %q", narrowOut)
	}
}

func TestHeaderDegradesOnNarrowWidth(t *testing.T) {
	narrow := newTestApp(30, 24)
	out := narrow.renderHeader()
	if lipgloss.Width(out) > 30 {
		t.Errorf("header must not exceed terminal width, got width %d", lipgloss.Width(out))
	}
	if !strings.Contains(out, "Atlas") {
		t.Error("expected the short 'Atlas' title to survive on a very narrow header")
	}
}

func TestWelcomeScreenListsRegisteredTools(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(fakeToolNamed{"read_file"})
	reg.Register(fakeToolNamed{"run_shell"})

	ag := agent.New(&fakeProvider{model: "fake-model"}, "", reg, false)
	app := New(ag, nil)
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	out := app.View()
	if !strings.Contains(out, "Araçlar") {
		t.Error("expected the session-info panel to show an 'Araçlar' accordion")
	}
	if !strings.Contains(out, "read_file") || !strings.Contains(out, "run_shell") {
		t.Errorf("expected registered tool names in the welcome panel, got:\n%s", out)
	}
}

type fakeToolNamed struct{ name string }

func (f fakeToolNamed) Name() string                 { return f.name }
func (f fakeToolNamed) Description() string          { return "" }
func (f fakeToolNamed) InputSchema() json.RawMessage { return json.RawMessage(`{}`) }
func (f fakeToolNamed) RequiresApproval() bool       { return false }
func (f fakeToolNamed) Execute(ctx context.Context, input json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}

func TestQueuedMessagesPreviewShowsQueuedText(t *testing.T) {
	app := newTestApp(80, 24)
	app.queuedMessages = []string{"birinci mesaj", "ikinci mesaj"}

	out := app.View()
	if !strings.Contains(out, "birinci mesaj") || !strings.Contains(out, "ikinci mesaj") {
		t.Errorf("expected both queued messages previewed, got:\n%s", out)
	}
}

func TestScrollbarRendersWithoutPanicWhenContentFitsAndOverflows(t *testing.T) {
	app := newTestApp(80, 10)
	// Short content: fits entirely, scrollbar should be a blank track.
	if out := app.renderScrollbar(app.chat.Height); lipgloss.Height(out) != app.chat.Height {
		t.Errorf("expected scrollbar height to match viewport height, got %d want %d", lipgloss.Height(out), app.chat.Height)
	}

	// Long content: forces scrolling, thumb must still fit within bounds.
	for i := 0; i < 200; i++ {
		app.messages = append(app.messages, chatMessage{role: "info", text: "satır"})
	}
	app.render()
	out := app.renderScrollbar(app.chat.Height)
	if lipgloss.Height(out) != app.chat.Height {
		t.Errorf("expected scrollbar height to match viewport height even when scrolling, got %d want %d", lipgloss.Height(out), app.chat.Height)
	}
}

func TestViewShowsCommandMenuWhileTypingSlash(t *testing.T) {
	app := newTestApp(80, 24)
	app.input.SetValue("/mo")
	out := app.View()

	if !strings.Contains(out, "/model") {
		t.Errorf("expected the slash-command menu to suggest /model, got:\n%s", out)
	}
}
