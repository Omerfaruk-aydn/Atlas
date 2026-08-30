package tui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/omerfarukaydin/atlas/internal/agent"
	"github.com/omerfarukaydin/atlas/internal/llm"
	"github.com/omerfarukaydin/atlas/internal/tools"
)

// fakeAgentForSlash is a minimal llm.Provider stand-in for slash-command
// tests. Re-uses the same shape as the existing newTestApp fake.
type fakeAgentForSlash struct{ model string }

func (f *fakeAgentForSlash) Name() string          { return "fake" }
func (f *fakeAgentForSlash) Model() string         { return f.model }
func (f *fakeAgentForSlash) SetModel(m string)     { f.model = m }
func (f *fakeAgentForSlash) ListModels(_ context.Context) ([]string, error) {
	return []string{f.model}, nil
}
func (f *fakeAgentForSlash) StreamChat(_ context.Context, _ llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}

type fakeToolForSlash struct{ name string }

func (f fakeToolForSlash) Name() string                       { return f.name }
func (f fakeToolForSlash) Description() string                { return "" }
func (f fakeToolForSlash) InputSchema() json.RawMessage       { return json.RawMessage(`{}`) }
func (f fakeToolForSlash) RequiresApproval() bool             { return false }
func (f fakeToolForSlash) Execute(_ context.Context, _ json.RawMessage) (tools.Result, error) {
	return tools.Result{}, nil
}

func newAgentForTest() *agent.Agent {
	reg := tools.NewRegistry()
	reg.Register(fakeToolForSlash{name: "x"})
	return agent.New(&fakeAgentForSlash{model: "fake-model"}, "", reg, false)
}

func newTestAppWithAgent(width, height int) *App {
	ag := newAgentForTest()
	app := New(ag, nil)
	app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return app
}

func TestNewCommandClearsState(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	app.messages = []chatMessage{{role: "user", text: "hi"}}
	app.sessionInTok = 100
	app.sessionOutTok = 50
	app.lastTurnMS = 200
	for _, c := range miscCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "new" {
			c.Run("", app)
		}
	}
	if len(app.messages) != 0 {
		t.Error("expected messages cleared after /new")
	}
	if app.sessionInTok != 0 || app.sessionOutTok != 0 {
		t.Error("expected token counters reset after /new")
	}
}

func TestBusyCommandSetsMode(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	for _, c := range sessionExtraCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "busy" {
			c.Run("steer", app)
			if app.busyMode != BusySteer {
				t.Errorf("expected BusySteer, got %q", app.busyMode)
			}
			c.Run("interrupt", app)
			if app.busyMode != BusyInterrupt {
				t.Errorf("expected BusyInterrupt, got %q", app.busyMode)
			}
		}
	}
}

func TestBranchCommandReportsGitBranch(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	app.gitBranch = "main"
	for _, c := range miscCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "branch" {
			note, _, _ := c.Run("", app)
			if !strings.Contains(note, "main") {
				t.Errorf("expected branch 'main' in output, got %q", note)
			}
		}
	}
}

func TestUsageCommandReportsTokens(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	app.sessionInTok = 1234
	app.sessionOutTok = 567
	for _, c := range miscCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "usage" {
			note, _, _ := c.Run("", app)
			if !strings.Contains(note, "giriş") || !strings.Contains(note, "çıkış") {
				t.Errorf("expected giriş/çıkış in usage, got %q", note)
			}
		}
	}
}

func TestDetailsCommandCyclesModes(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	for _, c := range sessionCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "details" {
			start := app.details.Global
			c.Run("", app)
			if app.details.Global == start {
				t.Error("expected details.Global to change after /details")
			}
			c.Run("hidden", app)
			if app.details.Global != DetailsHidden {
				t.Error("expected /details hidden to set DetailsHidden")
			}
		}
	}
}

func TestDebugDetailedToggles(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	start := app.detailedDebug
	for _, c := range sessionExtraCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "debug-detailed" {
			c.Run("", app)
			if app.detailedDebug == start {
				t.Error("expected detailedDebug to flip")
			}
		}
	}
}

func TestSessionsCommandOpensOverlay(t *testing.T) {
	app := newTestAppWithAgent(80, 24)
	app.messageHistory = []string{"first", "second"}
	for _, c := range coreCommands(&appForRegistry{ag: app.agent}) {
		if c.Name == "sessions" {
			c.Run("", app)
			if app.sessionSwitcher == nil {
				t.Error("expected sessionSwitcher to be opened by /sessions")
			}
		}
	}
}

func TestSlashRegistryFindsNewCommand(t *testing.T) {
	ag := newAgentForTest()
	app := New(ag, nil)
	for _, name := range []string{"new", "copy", "paste", "retry", "branch", "usage", "logs"} {
		if app.slash.Find(name) == nil {
			t.Errorf("expected /%s in the registry", name)
		}
	}
}

func TestSlashRegistryFindsSessionExtraCommands(t *testing.T) {
	ag := newAgentForTest()
	app := New(ag, nil)
	for _, name := range []string{"theme", "reasoning", "busy", "compress", "voice", "pet", "subagents", "rollback", "replay", "skills", "plugins", "debug-detailed"} {
		if app.slash.Find(name) == nil {
			t.Errorf("expected /%s in the registry", name)
		}
	}
}
