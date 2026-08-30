package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ApprovalOption is one row in the multi-option approval prompt. The
// Kind drives the agent-side action; Label/Desc drive the display. The
// active "always" option is filtered at construction time by
// NewApprovalOptions when the agent signals smart-deny or tirith-warning
// modes — Atlas doesn't have those flags yet, but the seam is there.
type ApprovalOption struct {
	Kind  string // "once" | "session" | "always" | "deny"
	Label string
	Desc  string
}

// NewApprovalOptions builds the default 4-option approval menu. The
// "always" row is conditionally dropped (smartDenied/tirithWarning are
// both false in Atlas currently; the seam is here for when those flags
// land).
func NewApprovalOptions(smartDenied, tirithWarning bool) []ApprovalOption {
	opts := []ApprovalOption{
		{Kind: "once", Label: "[1] Bir kez", Desc: "Yalnızca bu çağrıyı onayla"},
		{Kind: "session", Label: "[2] Oturum", Desc: "Bu oturum boyunca aynı aracı onayla"},
	}
	if !tirithWarning {
		opts = append(opts, ApprovalOption{Kind: "always", Label: "[3] Her zaman", Desc: "Bu aracı kalıcı olarak onayla"})
	}
	denyDesc := "Reddet"
	if smartDenied {
		// smartDenied narrows the deny path to a "back to model" loop
		// where the model can revise the call.
		denyDesc = "Reddet · model düzeltsin"
		opts = []ApprovalOption{
			{Kind: "once", Label: "[1] Bir kez", Desc: "Yalnızca bu çağrıyı onayla"},
			{Kind: "deny", Label: "[2] " + denyDesc, Desc: ""},
		}
		return opts
	}
	opts = append(opts, ApprovalOption{Kind: "deny", Label: fmt.Sprintf("[%d] Reddet", len(opts)+1), Desc: "Bu aracı çalıştırma"})
	return opts
}

// approvalKeyAction is the pure key-dispatch function for the multi-
// option prompt. Returns one of: choose(idx) for an Enter/quick-pick,
// move(delta) for arrow nav, noop for an unrelated key. Keeping it pure
// (no Bubbletea imports) means the same logic is trivially testable.
type approvalAction struct {
	kind string // "choose" | "move" | "noop" | "cancel"
	idx  int
	delta int
}

func approvalKeyAction(key string, sel int, opts []ApprovalOption) approvalAction {
	if len(opts) == 0 {
		return approvalAction{kind: "noop"}
	}
	switch key {
	case "up", "k":
		next := sel - 1
		if next < 0 {
			next = len(opts) - 1
		}
		return approvalAction{kind: "move", delta: next - sel, idx: next}
	case "down", "j", "tab":
		next := sel + 1
		if next >= len(opts) {
			next = 0
		}
		return approvalAction{kind: "move", delta: next - sel, idx: next}
	case "enter":
		return approvalAction{kind: "choose", idx: sel}
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Number quick-pick. Map the digit to the option at that
		// position (1-indexed).
		idx := int(key[0] - '1')
		if idx < 0 || idx >= len(opts) {
			return approvalAction{kind: "noop"}
		}
		return approvalAction{kind: "choose", idx: idx}
	case "y":
		// "y" is the legacy one-key approve. Map to the first
		// non-deny option, or just the first option.
		for i, o := range opts {
			if o.Kind != "deny" {
				return approvalAction{kind: "choose", idx: i}
			}
		}
		return approvalAction{kind: "choose", idx: 0}
	case "n", "esc", "ctrl+c":
		return approvalAction{kind: "cancel"}
	}
	return approvalAction{kind: "noop"}
}

// renderApprovalPrompt paints the multi-option approval box: double-
// border, "⚠ onay gerekli · <desc>" header, the tool input word-wrapped
// to the box width and capped to 10 lines (with a "+N more lines"
// footer), then the options as numbered rows with a "▸"/"  " cursor
// and a chip-style background on the active row.
func (a *App) renderApprovalPrompt(req *approvalRequest) string {
	width := a.width - 4
	if width < 30 {
		width = 30
	}

	var b strings.Builder
	b.WriteString(a.theme.ErrorText.Render("⚠ onay gerekli: " + req.toolName))
	b.WriteString("\n")

	// Body: diff preview if available, else pretty-printed JSON.
	if req.hasPreview() {
		header := req.previewPath
		if req.previewOld == "" {
			header += " (yeni dosya)"
		}
		b.WriteString(a.theme.UserLabel.Render(header))
		b.WriteString("\n")
		b.WriteString(a.renderDiff(req.previewOld, req.previewNew))
	} else {
		raw := string(req.input)
		if pretty, err := json.MarshalIndent(json.RawMessage(req.input), "", "  "); err == nil {
			raw = string(pretty)
		}
		wrapped := lipgloss.NewStyle().Width(width - 4).Render(raw)
		b.WriteString(a.theme.HelpText.Render(capLines(wrapped, 10)))
	}

	// Options list.
	opts := NewApprovalOptions(false, false)
	b.WriteString("\n")
	// Compute column widths.
	labelW := 0
	for _, o := range opts {
		if w := lipgloss.Width(o.Label); w > labelW {
			labelW = w
		}
	}
	for i, o := range opts {
		cursor := "  "
		style := a.theme.HelpText
		if i == a.approvalSelected {
			cursor = "▸ "
			style = a.theme.UserLabel
		}
		line := fmt.Sprintf("%s%-*s  %s", cursor, labelW, o.Label, o.Desc)
		if i == a.approvalSelected {
			// Chip-style background fill for the active row, matching
			// the picker treatment — not ANSI reverse (which renders
			// inconsistently on transparent backgrounds).
			b.WriteString(a.theme.SelectedBgBackground(style.Render(line)))
		} else {
			b.WriteString(style.Render(line))
		}
		if i < len(opts)-1 {
			b.WriteString("\n")
		}
	}

	// Footer hint.
	b.WriteString("\n")
	b.WriteString(a.theme.HelpText.Render("↑/↓ seç · Enter onayla · 1-" + fmt.Sprintf("%d", len(opts)) + " hızlı seçim · Esc/ctrl+c reddet"))

	return a.theme.InputBox.Width(width).Render(b.String())
}
