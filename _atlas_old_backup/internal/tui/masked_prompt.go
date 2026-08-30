package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MaskedPrompt is a secret-entry dialog. The visible echo is a run of
// "•" bullets (one per typed character), capped at 40 to keep the
// dialog from growing horizontally. Mirrors Hermes's maskedPrompt.tsx
// used for sudo/secret-env prompts.
type MaskedPrompt struct {
	Header     string
	SubLabel   string
	Masked     string // the actual secret (never rendered)
	Display    string // the visible bullet run
	CursorBlink bool
}

// renderMaskedPrompt paints the masked input. The cursor is a "▏"
// block at the end of the bullets, kept visible by the caller's
// Blink timer (or static if CursorBlink is false).
func (a *App) renderMaskedPrompt(p MaskedPrompt) string {
	width := a.width - 4
	if width < 30 {
		width = 30
	}
	var b strings.Builder
	if p.SubLabel != "" {
		b.WriteString(a.theme.Title.Render(p.Header))
		b.WriteString("\n")
		b.WriteString(a.theme.HelpText.Render(p.SubLabel))
		b.WriteString("\n\n")
	} else {
		b.WriteString(a.theme.Title.Render(p.Header))
		b.WriteString("\n\n")
	}
	masked := p.Display
	if len(masked) > 40 {
		masked = masked[:40]
	}
	b.WriteString(lipgloss.NewStyle().Width(width-4).Render(masked + "▏"))
	b.WriteString("\n\n")
	b.WriteString(a.theme.HelpText.Render("Gizli anahtar ~/.atlas/.env'e yazılacak. Esc iptal."))
	return a.theme.InputBox.Width(width).Render(b.String())
}

// maskedKeyAction dispatches one key for the masked prompt. Like
// modelPicker's API-key stage, but a separate primitive because the
// caller might want different display behavior (e.g. SSH password vs
// API key).
func maskedKeyAction(p MaskedPrompt, key string) (MaskedPrompt, string) {
	next := p
	switch key {
	case "enter":
		return next, "submit"
	case "esc", "ctrl+c":
		return next, "cancel"
	case "backspace":
		if len(next.Masked) > 0 {
			next.Masked = next.Masked[:len(next.Masked)-1]
			if strings.HasSuffix(next.Display, "•") {
				next.Display = next.Display[:len(next.Display)-len("•")]
			}
		}
		return next, "edit"
	}
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		next.Masked += key
		if len(next.Display) < 40 {
			next.Display += "•"
		}
		return next, "edit"
	}
	return next, "noop"
}
