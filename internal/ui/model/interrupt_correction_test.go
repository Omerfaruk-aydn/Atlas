package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	uv "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ultraviolet"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-widgets/v2/textarea"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/session"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/dialog"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

func TestTruncateInterruptedContentLeavesShortTextAlone(t *testing.T) {
	require.Equal(t, "hello", truncateInterruptedContent("  hello  \n"))
}

func TestTruncateInterruptedContentCapsLength(t *testing.T) {
	long := strings.Repeat("x", maxInterruptedContentChars+50)
	got := truncateInterruptedContent(long)
	require.Less(t, len([]rune(got)), len([]rune(long)))
	require.True(t, strings.HasSuffix(got, "…"))
}

// interruptCorrectionWorkspace answers just enough of
// interruptWithCorrection's calls (AgentCancel, ListMessages) to drive
// it without a real backend.
type interruptCorrectionWorkspace struct {
	workspace.Workspace
	cancelled []string
	messages  []message.Message
}

func (w *interruptCorrectionWorkspace) AgentCancel(sessionID string) {
	w.cancelled = append(w.cancelled, sessionID)
}

func (w *interruptCorrectionWorkspace) ListMessages(context.Context, string) ([]message.Message, error) {
	return w.messages, nil
}

func (w *interruptCorrectionWorkspace) Config() *config.Config {
	return nil
}

func newInterruptCorrectionTestUI(ws *interruptCorrectionWorkspace, busy bool) *UI {
	com := common.DefaultCommon(ws)

	ta := textarea.New()
	ta.SetStyles(com.Styles.Editor.Textarea)
	ta.SetVirtualCursor(false)

	m := &UI{
		com:        com,
		status:     NewStatus(com, nil),
		chat:       NewChat(com, config.ScrollbarDefault),
		textarea:   ta,
		state:      uiChat,
		focus:      uiFocusEditor,
		width:      140,
		height:     45,
		dialog:     dialog.NewOverlay(),
		session:    &session.Session{ID: "s1"},
		agentReady: true,
	}
	if busy {
		m.agentBusyCache = ttlCache[bool]{val: true, at: time.Now()}
	}
	return m
}

func assistantMessage(text string) message.Message {
	return message.Message{
		Role:  message.Assistant,
		Parts: []message.ContentPart{message.TextContent{Text: text}},
	}
}

func TestInterruptWithCorrectionNoOpWhenIdle(t *testing.T) {
	t.Parallel()

	ws := &interruptCorrectionWorkspace{}
	m := newInterruptCorrectionTestUI(ws, false)

	require.Nil(t, m.interruptWithCorrection())
	require.Empty(t, ws.cancelled)
}

func TestInterruptWithCorrectionNoOpWithoutSession(t *testing.T) {
	t.Parallel()

	ws := &interruptCorrectionWorkspace{}
	m := newInterruptCorrectionTestUI(ws, true)
	m.session = nil

	require.Nil(t, m.interruptWithCorrection())
}

// runInterruptedContentFetch runs interruptWithCorrection's returned
// batch and applies whichever sub-command yields interruptedContentMsg
// to m.Update. Other sub-commands in the same batch (e.g. the busy-state
// refresh doCancelAgent also schedules) touch Workspace methods this
// test's minimal fake does not implement and are not the point of this
// test, so a panic from one of those is swallowed rather than run
// through the fake's embedded nil Workspace.
func runInterruptedContentFetch(t *testing.T, m *UI, cmd tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd)
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "expected a batch of cancel + fetch commands")

	for _, c := range batch {
		if c == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			if msg, ok := c().(interruptedContentMsg); ok {
				m.Update(msg)
			}
		}()
	}
}

func TestInterruptWithCorrectionCancelsAndFillsComposer(t *testing.T) {
	t.Parallel()

	ws := &interruptCorrectionWorkspace{messages: []message.Message{
		assistantMessage("Here is the partial plan I was writing"),
	}}
	m := newInterruptCorrectionTestUI(ws, true)

	cmd := m.interruptWithCorrection()
	require.Equal(t, []string{"s1"}, ws.cancelled, "cancel must fire synchronously, not deferred into the returned command")

	runInterruptedContentFetch(t, m, cmd)

	require.Contains(t, m.textarea.Value(), "Here is the partial plan I was writing")
	require.Contains(t, m.textarea.Value(), "Correction: ")
}

func TestInterruptWithCorrectionIgnoresNonAssistantLastMessage(t *testing.T) {
	t.Parallel()

	ws := &interruptCorrectionWorkspace{messages: []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.TextContent{Text: "do the thing"}}},
	}}
	m := newInterruptCorrectionTestUI(ws, true)

	cmd := m.interruptWithCorrection()
	runInterruptedContentFetch(t, m, cmd)

	require.Empty(t, m.textarea.Value(), "a non-assistant last message must not be quoted into the composer")
}

// interruptCorrectionActionDialog turns a tea.MouseClickMsg into
// dialog.ActionInterruptWithCorrection, the same trick
// freshSessionActionDialog uses, so the dispatch case in
// handleDialogMsg can be exercised through a real Update call.
type interruptCorrectionActionDialog struct{}

func (*interruptCorrectionActionDialog) ID() string { return "interrupt-correction-action" }

func (*interruptCorrectionActionDialog) HandleMsg(msg tea.Msg) dialog.Action {
	if _, ok := msg.(tea.MouseClickMsg); !ok {
		return nil
	}
	return dialog.ActionInterruptWithCorrection{}
}

func (*interruptCorrectionActionDialog) Draw(uv.Screen, uv.Rectangle) *tea.Cursor { return nil }

var _ dialog.Dialog = (*interruptCorrectionActionDialog)(nil)

func TestActionInterruptWithCorrectionDispatch(t *testing.T) {
	t.Parallel()

	ws := &interruptCorrectionWorkspace{messages: []message.Message{
		assistantMessage("partial output"),
	}}
	m := newInterruptCorrectionTestUI(ws, true)
	m.dialog.OpenDialog(&interruptCorrectionActionDialog{})

	_, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{Button: tea.MouseLeft}))
	require.NotNil(t, cmd)
	require.Equal(t, []string{"s1"}, ws.cancelled)
}
