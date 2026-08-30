package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PagerState is the state of one Pager overlay. Pager is the
// long-content viewer Hermes uses for help text, command output dumps,
// and any other "this doesn't fit in the chat" content. The App owns at
// most one Pager at a time; opening a new one closes the previous.
type PagerState struct {
	Title   string
	Content string
	Offset  int // top visible line index
}

// renderPager returns the rendered pager overlay. The footer hint
// dynamically changes depending on whether more content remains above or
// below the current offset.
func (a *App) renderPager(p PagerState) string {
	lines := strings.Split(p.Content, "\n")
	width := a.width - 4
	if width < 20 {
		width = 20
	}
	height := a.height - 8
	if height < 5 {
		height = 5
	}

	// Clamp offset.
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Offset > len(lines)-1 {
		p.Offset = len(lines) - 1
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	end := p.Offset + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[p.Offset:end]

	// Title (centered) + body + footer hint.
	title := lipgloss.NewStyle().Bold(true).Width(width).Align(lipgloss.Center).Render(p.Title)
	body := strings.Join(visible, "\n")
	hint := pagerHint(p.Offset, end, len(lines))
	hintRendered := lipgloss.NewStyle().Faint(true).Width(width).Align(lipgloss.Center).Render(hint)

	boxed := a.theme.InputBox.Width(width).Render(strings.Join([]string{title, body, hintRendered}, "\n"))
	return boxed
}

// pagerHint returns the footer string for the current pager position.
// Matches Hermes: "↑↓/jk line · Enter/Space/PgDn page · b/PgUp back ·
// q close" mid-content; switches to "q close · end" when at the bottom;
// "q close" when at the very top.
func pagerHint(offset, end, total int) string {
	atTop := offset == 0
	atBottom := end >= total
	switch {
	case atTop && atBottom:
		return "q kapat"
	case atTop:
		return "↓/j satır · Enter/Space/PgDn sayfa · q kapat"
	case atBottom:
		return "↑↓/jk satır · b/PgUp sayfa · q kapat · son"
	default:
		return "↑↓/jk satır · Enter/Space/PgDn sayfa · b/PgUp sayfa · q kapat"
	}
}

// PagerKeyResult describes what the App should do after a key press in
// the pager.
type PagerKeyResult int

const (
	PagerContinue PagerKeyResult = iota
	PagerClose
)

// handlePagerKey applies one tea.KeyMsg to the pager state. Returns
// (new state, result). Keeping the dispatch pure (no Bubbletea imports)
// makes the same logic trivially unit-testable.
func handlePagerKey(p PagerState, key string) (PagerState, PagerKeyResult) {
	lines := strings.Split(p.Content, "\n")
	maxOffset := len(lines) - 1
	if maxOffset < 0 {
		return p, PagerClose
	}
	height := 5
	switch key {
	case "q", "esc", "Q":
		return p, PagerClose
	case "down", "j":
		if p.Offset >= maxOffset {
			return p, PagerContinue
		}
		p.Offset++
	case "up", "k":
		if p.Offset <= 0 {
			return p, PagerContinue
		}
		p.Offset--
	case "pagedown", "space", "enter":
		if p.Offset >= maxOffset {
			return p, PagerContinue
		}
		p.Offset += height
		if p.Offset > maxOffset {
			p.Offset = maxOffset
		}
	case "pageup", "b":
		if p.Offset <= 0 {
			return p, PagerContinue
		}
		p.Offset -= height
		if p.Offset < 0 {
			p.Offset = 0
		}
	case "home", "g":
		p.Offset = 0
	case "end", "G":
		p.Offset = maxOffset
	default:
		_ = fmt.Sprintf("unhandled pager key: %s", key)
	}
	return p, PagerContinue
}
