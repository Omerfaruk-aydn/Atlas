package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// roleStyle is the per-role rendering table. Mirrors Hermes's
// domain/roles.ts ROLE lookup, which returns (body, glyph, prefix)
// per role. Atlas stores the per-role glyph + color tier; the
// transcript renderer in app.go uses these instead of hardcoded
// literals so a future skin change propagates to every role.
type roleStyle struct {
	glyph string
	color lipgloss.AdaptiveColor
	prefixColor lipgloss.AdaptiveColor
	boxed bool // render in a bordered round box (tool role)
}

func (a *App) roleTable() map[string]roleStyle {
	return map[string]roleStyle{
		"assistant": {
			glyph:       a.theme.AsstGlyph,
			color:       adaptFromColor(a.theme.Asst),
			prefixColor: adaptFromColor(a.theme.Asst),
		},
		"user": {
			glyph:       a.theme.UserGlyph,
			color:       adaptFromColor(a.theme.User),
			prefixColor: adaptFromColor(a.theme.User),
		},
		"system": {
			glyph:       "·",
			color:       a.theme.Muted,
			prefixColor: a.theme.Muted,
		},
		"tool": {
			glyph:       "⚡",
			color:       a.theme.Muted,
			prefixColor: a.theme.Muted,
			boxed:       true,
		},
		"event": {
			glyph:       "◈",
			color:       a.theme.Muted,
			prefixColor: a.theme.Muted,
		},
	}
}

// adaptFromColor lifts a fixed lipgloss.Color into an AdaptiveColor by
// using the same hex for both polarities. Used by roleTable to feed
// the AdaptiveColor-shaped style fields.
func adaptFromColor(c lipgloss.Color) lipgloss.AdaptiveColor {
	s := string(c)
	return lipgloss.AdaptiveColor{Dark: s, Light: s}
}

// renderRoleHeader draws the "[HH:MM] Role" prefix row above a
// message. The timestamp is rendered on its own row above the message
// (NOT inline) per Hermes's display.timestamps implementation. Only
// user / assistant / tool messages get a timestamp; events and system
// chrome do not.
func (a *App) renderRoleHeader(role string, t time.Time) string {
	rs, ok := a.roleTable()[role]
	if !ok {
		rs = a.roleTable()["system"]
	}
	stamp := ""
	if t != (time.Time{}) && (role == "user" || role == "assistant" || role == "tool") {
		stamp = a.theme.HelpText.Render(fmt.Sprintf("[%s]", t.Format("15:04")))
	}
	labelStyle := lipgloss.NewStyle().Foreground(rs.color).Bold(true)
	glyph := rs.glyph
	label := ""
	switch role {
	case "user":
		label = "Sen"
	case "assistant":
		label = "Atlas"
	default:
		label = role
	}
	row := labelStyle.Render(glyph + " " + label)
	if stamp != "" {
		return stamp + "\n" + row
	}
	return row
}

// renderLongMessage handles messages that exceed the longMessageCharLimit
// (300 chars in Hermes's config/limits.ts). A long message collapses
// to a short label: first line, first 4 words (or the whole first line
// if one word), max 80 chars, suffixed with [long message].
//
// The full text is preserved internally — the renderer just shows the
// short label in the collapsed state. The caller (typically a "press
// Enter to expand" handler) decides when to flip to expanded.
const longMessageCharLimit = 300
const longMessageLabelMax = 80

func collapsedLongMessageLabel(text string) string {
	firstLine := text
	if nl := strings.Index(text, "\n"); nl >= 0 {
		firstLine = text[:nl]
	}
	words := strings.Fields(firstLine)
	head := strings.Join(words, " ")
	if len(head) > longMessageLabelMax {
		head = head[:longMessageLabelMax-1] + "…"
	}
	return head + " [long message]"
}

// longMessageFolded returns (collapsed, expanded) representations of a
// long user message. The caller renders the collapsed form by default
// and the expanded form on expand.
func longMessageFolded(text string) (collapsed, expanded string) {
	if len(text) <= longMessageCharLimit {
		return text, text
	}
	return collapsedLongMessageLabel(text), text
}

// responseDividerText returns the "└─ Response" connector line that
// appears between a collapsed reasoning/tools trail and the visible
// assistant reply. Without it, a user reading the transcript can't
// tell that the response they see is *the* response to the prompt
// above, and not just another random message.
func (a *App) responseDividerText() string {
	return lipgloss.NewStyle().Foreground(a.theme.Border).Render("└─ Response")
}
