package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/omerfarukaydin/atlas/internal/agent"
)

// agentEventMsg wraps one agent.Event as it arrives off the turn channel.
type agentEventMsg struct {
	ev agent.Event
	ch <-chan agent.Event
}

// agentDoneMsg signals the turn's event channel has closed.
type agentDoneMsg struct{}

// modelsListMsg carries the result of an async ListModels call triggered by
// the "/model" command with no arguments.
type modelsListMsg struct {
	models []string
	err    error
}

// renderTickMsg is the periodic coalesce tick that decides whether to
// repaint the streaming assistant bubble. Adaptive delay (16/80/96ms)
// is honored by re-arming the tick in the App's renderTickMsg case.
type renderTickMsg struct{}

// renderTick returns a tea.Cmd that fires after delay and produces a
// renderTickMsg. The App re-arms from the case so the interval can
// adapt to user activity.
func renderTick() tea.Cmd {
	return tea.Tick(renderTickInterval, func(time.Time) tea.Msg { return renderTickMsg{} })
}

// faceTickMsg advances the busy indicator. Re-arms itself every
// faceTickMS so the App's Update loop can re-issue the next tick.
type faceTickMsg struct{}

// faceTick returns a tea.Cmd that fires once and produces a faceTickMsg.
// The App re-arms it from the faceTickMsg case in Update.
func faceTick() tea.Cmd {
	return tea.Tick(faceTickInterval, func(time.Time) tea.Msg { return faceTickMsg{} })
}

// shimmerTickMsg drives the shimmer shared clock forward.
type shimmerTickMsg struct{}

// shimmerTick schedules the next shimmer broadcast.
func shimmerTick() tea.Cmd {
	return tea.Tick(shimmerTickInterval, func(time.Time) tea.Msg { return shimmerTickMsg{} })
}

// waitForAgentEvent drains one event off ch and reports it as a tea.Msg,
// carrying the channel forward so Update can keep listening. This is the
// classic Bubbletea "wait on a channel without blocking the UI" pattern.
func waitForAgentEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentDoneMsg{}
		}
		return agentEventMsg{ev: ev, ch: ch}
	}
}

// completionFilterMsg carries the result of an async slash-command
// fuzzy search, so completion-flicker races against the user's typing
// can be filtered with a "flight counter" guard.
type completionFilterMsg struct {
	seq     int64
	results []FuzzyScoreItem
}

// backgroundDetectMsg carries the resolved background polarity after the
// first OSC-11 probe resolves. (We don't actually issue the probe yet —
// it's a stub slot for when we add the termprobe wiring. Keeping the
// type so the App's Update case is ready.)
type backgroundDetectMsg struct {
	mode LightMode
}
