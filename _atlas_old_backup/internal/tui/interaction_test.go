package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeText(app *App, s string) {
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
}

func TestEnterWhileStreamingQueuesInsteadOfBlocking(t *testing.T) {
	app := newTestApp(80, 24)
	app.streaming = true

	typeText(app, "sonraki mesaj")
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(app.queuedMessages) != 1 || app.queuedMessages[0] != "sonraki mesaj" {
		t.Fatalf("expected message to be queued, got %v", app.queuedMessages)
	}
	if app.input.Value() != "" {
		t.Error("expected input to clear after queueing")
	}
	// Queueing must not start a second turn on top of the running one.
	if !app.streaming {
		t.Error("expected streaming to remain true (original turn still running)")
	}
}

func TestQueuedMessageDrainsWhenTurnEnds(t *testing.T) {
	app := newTestApp(80, 24)
	app.streaming = true
	app.queuedMessages = []string{"kuyruktaki mesaj"}

	model, _ := app.Update(agentDoneMsg{})
	app = model.(*App)

	if len(app.queuedMessages) != 0 {
		t.Errorf("expected queue to drain, still has %v", app.queuedMessages)
	}
	if !app.streaming {
		t.Error("expected a new turn to start automatically for the queued message")
	}
	// The queued text should have become the latest user message.
	var found bool
	for _, m := range app.messages {
		if m.role == "user" && m.text == "kuyruktaki mesaj" {
			found = true
		}
	}
	if !found {
		t.Error("expected the queued message to appear as a user message")
	}
}

func TestCtrlCPriorityClearsDraftBeforeQuitting(t *testing.T) {
	app := newTestApp(80, 24)
	typeText(app, "bitmemiş taslak")

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	app = model.(*App)

	if cmd != nil {
		t.Error("expected Ctrl+C to just clear the draft, not quit, when input is non-empty")
	}
	if app.input.Value() != "" {
		t.Errorf("expected draft to be cleared, got %q", app.input.Value())
	}
}

func TestCtrlCQuitsWhenDraftEmpty(t *testing.T) {
	app := newTestApp(80, 24)

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected Ctrl+C to return a quit command when the draft is already empty")
	}
}

func TestCtrlCCancelsStreamingTurnFirst(t *testing.T) {
	app := newTestApp(80, 24)
	app.streaming = true
	canceled := false
	app.cancel = func() { canceled = true }
	typeText(app, "bu taslak silinmemeli")

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	app = model.(*App)

	if !canceled {
		t.Error("expected Ctrl+C to cancel the in-flight turn first")
	}
	if cmd != nil {
		t.Error("expected no quit command on the cancel step")
	}
	if app.input.Value() != "bu taslak silinmemeli" {
		t.Error("expected the draft to survive the cancel step (only cleared on a second Ctrl+C)")
	}
}

func TestHistoryRecallOnlyWhenInputEmpty(t *testing.T) {
	app := newTestApp(80, 24)
	app.messageHistory = []string{"ilk mesaj", "ikinci mesaj"}

	app.Update(tea.KeyMsg{Type: tea.KeyUp})
	if app.input.Value() != "ikinci mesaj" {
		t.Fatalf("expected most recent message recalled first, got %q", app.input.Value())
	}

	app.Update(tea.KeyMsg{Type: tea.KeyUp})
	if app.input.Value() != "ilk mesaj" {
		t.Fatalf("expected older message on second Up, got %q", app.input.Value())
	}

	app.Update(tea.KeyMsg{Type: tea.KeyDown})
	if app.input.Value() != "ikinci mesaj" {
		t.Fatalf("expected Down to move to the newer message, got %q", app.input.Value())
	}

	app.Update(tea.KeyMsg{Type: tea.KeyDown})
	if app.input.Value() != "" {
		t.Fatalf("expected Down past the newest message to restore an empty draft, got %q", app.input.Value())
	}
}

func TestHistoryRecallDoesNotTriggerWithNonEmptyDraft(t *testing.T) {
	app := newTestApp(80, 24)
	app.messageHistory = []string{"geçmiş mesaj"}
	typeText(app, "yazmakta olduğum şey")

	app.Update(tea.KeyMsg{Type: tea.KeyUp})

	if app.input.Value() == "geçmiş mesaj" {
		t.Error("Up must not clobber a non-empty in-progress draft")
	}
}

func TestSubmitRecordsMessageHistory(t *testing.T) {
	app := newTestApp(80, 24)
	typeText(app, "kaydedilecek mesaj")
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(app.messageHistory) != 1 || app.messageHistory[0] != "kaydedilecek mesaj" {
		t.Errorf("expected the sent message to be recorded in history, got %v", app.messageHistory)
	}
}

func TestCapLinesAddsFooterWhenTruncated(t *testing.T) {
	long := strings.Repeat("satır\n", 20)
	out := capLines(strings.TrimRight(long, "\n"), 5)

	if strings.Count(out, "\n") != 5 {
		t.Errorf("expected 5 kept lines plus footer, got:\n%s", out)
	}
	if !strings.Contains(out, "+15 satır daha") {
		t.Errorf("expected a footer naming the remaining line count, got:\n%s", out)
	}
}

func TestCapLinesNoOpWhenShort(t *testing.T) {
	short := "bir\niki\nüç"
	if out := capLines(short, 10); out != short {
		t.Errorf("expected no change for content under the cap, got %q", out)
	}
}
