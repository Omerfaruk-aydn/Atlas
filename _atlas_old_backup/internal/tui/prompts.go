package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ClarifyPrompt is the batch-question prompt: the model can ask several
// questions at once; the user answers them one at a time with Tab /
// Shift+Tab cycling the active question. Atlas's port mirrors Hermes's
// ClarifyPrompt — questions slice, active index, answered map, and the
// universal "Other (type your answer)" row appended after the
// enumerated choices.
type ClarifyPrompt struct {
	Questions []ClarifyQuestion
	Active    int
	Answers   []string // parallel to Questions; "" = unanswered
	// OtherAnswerIndex is the row index (in the rendered options list)
	// of the "type your answer" row for the currently active question.
	OtherAnswerIndex int
}

// ClarifyQuestion is one question in a batch. Choices are the
// enumerated options; an empty slice means "free-form text only".
type ClarifyQuestion struct {
	Header  string
	Question string
	Choices []string
	Multi   bool
}

// clarifyStatusText returns the "answered/total · ↑/↓ select · Enter
// lock answer · Tab/Shift+Tab switch question · Esc/Ctrl+C cancel"
// footer with the live answered count. Mirrors Hermes's footer text.
func (c ClarifyPrompt) clarifyStatusText() string {
	answered := 0
	for _, a := range c.Answers {
		if a != "" {
			answered++
		}
	}
	return fmt.Sprintf("%d/%d yanıtlandı · ↑/↓ seç · Enter kilitle · Tab/Shift+Tab soru değiştir · Esc/Ctrl+C iptal",
		answered, len(c.Questions))
}

// renderClarify paints the batch-question modal. Active question is
// highlighted; answered questions show ✓ + their locked answer (or
// "(atlandı)" if skipped); untouched questions show ·.
func (a *App) renderClarify(c ClarifyPrompt) string {
	width := a.width - 4
	if width < 30 {
		width = 30
	}
	var b strings.Builder
	b.WriteString(a.theme.Title.Render("Birkaç soru var"))
	b.WriteString("\n\n")
	successStyle := lipgloss.NewStyle().Foreground(a.theme.Success).Bold(true)
	for i, q := range c.Questions {
		marker := "  "
		style := a.theme.HelpText
		switch {
		case c.Answers[i] != "" && c.Answers[i] != "(atlandı)":
			marker = "✓ "
			style = successStyle
		case c.Answers[i] == "(atlandı)":
			marker = "  "
			style = a.theme.HelpText
		case i == c.Active:
			marker = "▸ "
			style = a.theme.UserLabel
		default:
			marker = "· "
		}
		header := q.Header
		if header == "" {
			header = fmt.Sprintf("Soru %d", i+1)
		}
		b.WriteString(style.Render(marker + header + " — " + q.Question))
		if c.Answers[i] != "" {
			b.WriteString("\n    " + a.theme.HelpText.Render("→ " + c.Answers[i]))
		}
		if i == c.Active {
			// Enumerated choices for the active question.
			b.WriteString("\n")
			for j, choice := range q.Choices {
				b.WriteString("\n    " + a.theme.HelpText.Render(fmt.Sprintf("  [%d] %s", j+1, choice)))
			}
			b.WriteString("\n    " + a.theme.HelpText.Render(fmt.Sprintf("  [%d] Diğer (kendi cevabını yaz)", len(q.Choices)+1)))
			b.WriteString("\n    " + a.theme.HelpText.Render(fmt.Sprintf("  [s] Atla")))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(a.theme.HelpText.Render(c.clarifyStatusText()))
	return a.theme.InputBox.Width(width).Render(b.String())
}

// clarifyKeyAction dispatches one key event for the clarify prompt.
// Returns the new (active, answer, choiceIdx, action) tuple where
// action is "choose" | "next" | "prev" | "skip" | "other" | "cancel" | "noop".
type clarifyKeyResult struct {
	active   int
	answer   string
	choiceIdx int
	action   string
}

func clarifyKeyAction(c ClarifyPrompt, key string) clarifyKeyResult {
	res := clarifyKeyResult{active: c.Active, action: "noop"}
	if len(c.Questions) == 0 {
		return res
	}
	switch key {
	case "tab":
		res.active = (c.Active + 1) % len(c.Questions)
		res.action = "next"
	case "shift+tab":
		res.active = (c.Active - 1 + len(c.Questions)) % len(c.Questions)
		res.action = "prev"
	case "up":
		// (within active question, no-op for now — could power choice cycling)
		res.action = "noop"
	case "down":
		res.action = "noop"
	case "enter":
		// Lock the active question's first choice as the answer.
		if len(c.Questions[c.Active].Choices) > 0 {
			res.answer = c.Questions[c.Active].Choices[0]
			res.choiceIdx = 0
		} else {
			res.answer = "(atlandı)"
		}
		res.action = "choose"
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(key[0] - '1')
		q := c.Questions[c.Active]
		if idx < len(q.Choices) {
			res.answer = q.Choices[idx]
			res.choiceIdx = idx
			res.action = "choose"
		} else if idx == len(q.Choices) {
			res.action = "other"
		}
	case "s", "S":
		res.answer = "(atlandı)"
		res.action = "skip"
	case "esc", "ctrl+c":
		res.action = "cancel"
	}
	return res
}

// ConfirmPrompt is a Y/N dialog with an optional danger mode (red border
// + text) for destructive actions. Mirrors Hermes's ConfirmPrompt.
type ConfirmPrompt struct {
	Header  string
	Body    string
	Danger  bool
}

// renderConfirm paints the Y/N dialog.
func (a *App) renderConfirm(c ConfirmPrompt) string {
	width := a.width - 4
	if width < 30 {
		width = 30
	}
	var b strings.Builder
	headerStyle := a.theme.Title
	if c.Danger {
		headerStyle = a.theme.ErrorText
	}
	b.WriteString(headerStyle.Render(c.Header))
	b.WriteString("\n")
	if c.Body != "" {
		b.WriteString(a.theme.HelpText.Render(c.Body))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(a.theme.UserLabel.Render("[y] evet"))
	b.WriteString("   ")
	b.WriteString(a.theme.HelpText.Render("[n] hayır"))
	border := a.theme.InputBox
	if c.Danger {
		border = a.theme.ApprovalBox
	}
	return border.Width(width).Render(b.String())
}

// confirmKeyAction returns "yes" | "no" | "noop".
func confirmKeyAction(key string) string {
	switch key {
	case "y", "Y", "yes", "enter":
		return "yes"
	case "n", "N", "no", "esc", "ctrl+c":
		return "no"
	}
	return "noop"
}

// padString is a tiny string padding helper used by the prompt renderers.
func padString(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// renderPromptZone is the single-cell WidgetGrid Atlas uses to host
// whichever blocking prompt is active. It exists for layout-engine
// uniformity — every prompt is rendered inside the same-shaped cell so
// the App's View() can compose them with a single branch.
//
// In Hermes, PromptZone is a 1-column WidgetGrid; the Atlas port wraps
// the active prompt body in a box with the theme's standard padding.
func (a *App) renderPromptZone(body string) string {
	return lipgloss.NewStyle().Padding(0, 1).Render(body)
}
