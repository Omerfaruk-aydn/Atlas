package tui

import "github.com/charmbracelet/lipgloss"

// responseDividerText is a convenience wrapper that returns the
// "└─ Response" connector line shown between a collapsed reasoning
// and tools trail and the visible assistant reply. Without it, a
// user reading the transcript can't tell that the response they see
// is the response to the prompt above, and not just another random
// message. Mirrors Hermes's log-update.ts "└─ Response" treatment.
func (a *App) trailResponseDivider() string {
	return lipgloss.NewStyle().
		Foreground(a.theme.Border).
		Italic(true).
		Render("└─ Response")
}
