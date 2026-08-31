package model

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/ui/dialog"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/require"
)

type dialogMouseActionMsg struct{}

type mouseActionDialog struct{}

func (*mouseActionDialog) ID() string {
	return "mouse-action"
}

func (*mouseActionDialog) HandleMsg(msg tea.Msg) dialog.Action {
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return nil
	}
	return dialog.ActionCmd{Cmd: func() tea.Msg { return dialogMouseActionMsg{} }}
}

func (*mouseActionDialog) Draw(uv.Screen, uv.Rectangle) *tea.Cursor {
	return nil
}

func TestMouseClickExecutesDialogActionCommand(t *testing.T) {
	t.Parallel()

	m := newTestUI()
	m.dialog = dialog.NewOverlay()
	m.dialog.OpenDialog(new(mouseActionDialog))

	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{Button: tea.MouseLeft}))
	require.NotNil(t, cmd)
	requireBatchContains(t, cmd(), dialogMouseActionMsg{})
}

// requireBatchContains asserts that msg is either directly of the wanted
// type, or (since opening a dialog also schedules its backdrop dim-in tick,
// batching multiple commands together) a tea.BatchMsg containing a command
// that produces it.
func requireBatchContains(t *testing.T, msg tea.Msg, want any) {
	t.Helper()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		require.IsType(t, want, msg)
		return
	}
	for _, cmd := range batch {
		if cmd == nil {
			continue
		}
		if got := cmd(); got != nil {
			if _, matches := got.(dialogMouseActionMsg); matches {
				return
			}
		}
	}
	t.Fatalf("expected batch to contain a command producing %T, got %#v", want, batch)
}

var _ dialog.Dialog = (*mouseActionDialog)(nil)
