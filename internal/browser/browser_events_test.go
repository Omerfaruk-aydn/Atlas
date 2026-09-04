package browser

import (
	"testing"

	"github.com/chromedp/cdproto/runtime"
	"github.com/stretchr/testify/require"
)

func remoteObjectValue(literal string) *runtime.RemoteObject {
	return &runtime.RemoteObject{Value: []byte(literal)}
}

func TestFormatConsoleArgsPrefersLiteralValues(t *testing.T) {
	got := formatConsoleArgs([]*runtime.RemoteObject{
		remoteObjectValue(`"hello"`),
		remoteObjectValue("42"),
	})
	require.Equal(t, "hello 42", got)
}

func TestFormatConsoleArgsFallsBackToDescriptionThenClassName(t *testing.T) {
	got := formatConsoleArgs([]*runtime.RemoteObject{
		{Description: "Error: boom"},
		{ClassName: "HTMLDivElement"},
	})
	require.Equal(t, "Error: boom HTMLDivElement", got)
}

func TestFormatConsoleArgsSkipsNilEntries(t *testing.T) {
	got := formatConsoleArgs([]*runtime.RemoteObject{nil, remoteObjectValue(`"x"`)})
	require.Equal(t, "x", got)
}

func TestFormatConsoleArgsEmpty(t *testing.T) {
	require.Empty(t, formatConsoleArgs(nil))
}

func TestAppendConsoleCapsAtMaxEntries(t *testing.T) {
	s := &chromedpSession{}
	for i := 0; i < maxConsoleEntries+10; i++ {
		s.appendConsole(ConsoleEntry{Type: "log", Text: "entry"})
	}
	logs := s.ConsoleLogs()
	require.Len(t, logs, maxConsoleEntries, "the buffer must not grow past its cap")
}

func TestConsoleLogsReturnsACopy(t *testing.T) {
	s := &chromedpSession{}
	s.appendConsole(ConsoleEntry{Type: "log", Text: "first"})

	logs := s.ConsoleLogs()
	logs[0].Text = "mutated"

	require.Equal(t, "first", s.ConsoleLogs()[0].Text, "callers must not be able to mutate session state through the returned slice")
}

func TestAppendDialogCapsAtMaxPendingDialogs(t *testing.T) {
	s := &chromedpSession{}
	for i := 0; i < maxPendingDialogs+5; i++ {
		s.appendDialog(DialogInfo{Type: "alert", Message: "hi"})
	}
	require.Len(t, s.PendingDialogs(), maxPendingDialogs)
}

func TestHandleDialogErrorsWhenNothingIsPending(t *testing.T) {
	s := &chromedpSession{}
	err := s.HandleDialog(true, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no pending dialog")
}

func TestHandleDialogPopsInFIFOOrder(t *testing.T) {
	s := &chromedpSession{}
	s.appendDialog(DialogInfo{Type: "alert", Message: "first"})
	s.appendDialog(DialogInfo{Type: "confirm", Message: "second"})

	// HandleDialog's own s.run call would need a live browser target, so
	// this only exercises the FIFO bookkeeping: the dequeue must happen
	// (and happen in order) before that call is ever reached. A nil ctx
	// panic from the unreachable-in-this-test s.run is caught and
	// ignored -- the assertion is about what got dequeued, not about
	// completing the (unavailable here) CDP round trip.
	func() {
		defer func() { _ = recover() }()
		_ = s.HandleDialog(true, "")
	}()

	require.Equal(t, []DialogInfo{{Type: "confirm", Message: "second"}}, s.PendingDialogs(),
		"the oldest pending dialog must be dequeued first")
}
