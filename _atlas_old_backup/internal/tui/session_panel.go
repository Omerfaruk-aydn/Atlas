package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// SessionPanel is the big bordered round-box startup panel that
// Hermes shows before the first user message. It's the "Atlas is
// ready" splash — a hero header, info lines (provider/model/cwd/
// branch), and a few accordions for Tools / Skills / System-Prompt /
// MCP Servers. Atlas has no Skills/MCP layers yet, so those sections
// fall back to a "not configured" body when their backing data is
// empty.
type SessionPanel struct {
	Info       SessionInfo
	Tools      []string
	Skills     []string
	SystemPrompt string
	MCPServers []string
	Lazy       bool // true while Skills/MCP sections are still loading
}

// SessionInfo is the headline data shown in the panel header.
type SessionInfo struct {
	Provider string
	Model    string
	CWD      string
	Branch   string
	StartedAt time.Time
}

// renderSessionPanel paints the big startup panel. Layout (mirrors
// Hermes's SessionPanel two-column hero+info): a header, an info block
// (provider / model / cwd / branch), then accordions for Tools /
// Skills / System-Prompt / MCP Servers, with the Tools section open by
// default (matches Hermes's "open by default for live process feel")
// and the others collapsed.
func (a *App) renderSessionPanel(p SessionPanel) string {
	width := a.width - 4
	if width < 60 {
		width = 60
	}
	heroW := 28
	infoW := width - heroW - 6
	if infoW < 20 {
		infoW = 20
	}
	// Hero (left): Atlas title + tagline + maybe an update-nag.
	hero := lipgloss.NewStyle().
		Width(heroW).
		Padding(1, 2).
		Render(a.theme.Title.Render("Atlas") + "\n" +
			a.theme.HelpText.Render("terminal AI agent"))
	// Info (right): provider / model / cwd / branch / started.
	var infoLines []string
	infoLines = append(infoLines, fmt.Sprintf("sağlayıcı: %s", p.Info.Provider))
	infoLines = append(infoLines, fmt.Sprintf("model:     %s", p.Info.Model))
	if p.Info.CWD != "" {
		infoLines = append(infoLines, fmt.Sprintf("dizin:     %s", p.Info.CWD))
	}
	if p.Info.Branch != "" {
		infoLines = append(infoLines, fmt.Sprintf("dal:       %s", p.Info.Branch))
	}
	if !p.Info.StartedAt.IsZero() {
		infoLines = append(infoLines, fmt.Sprintf("başladı:   %s",
			p.Info.StartedAt.Format("15:04")))
	}
	info := a.theme.HelpText.Render(strings.Join(infoLines, "\n"))
	heroInfo := lipgloss.JoinHorizontal(lipgloss.Top, hero, info)

	// Accordions.
	var sections []string
	sections = append(sections, heroInfo)
	sections = append(sections, "")
	sections = append(sections, a.renderSessionAccordionBlock(p, width-4))

	return a.theme.InputBox.Width(width).Render(strings.Join(sections, "\n"))
}

// renderSessionAccordionBlock renders the Tools / Skills / System-Prompt /
// MCP accordions. Tools is open by default; the rest start collapsed.
// While Lazy is true, the Skills and MCP sections render a shimmer
// skeleton instead of "loading" text — same pattern as Hermes.
func (a *App) renderSessionAccordionBlock(p SessionPanel, width int) string {
	var b strings.Builder
	// Tools (open by default).
	toolsBody := ""
	if len(p.Tools) > 0 {
		toolsBody = a.theme.HelpText.Render("  " + truncLine(p.Tools, width-12))
	} else {
		toolsBody = a.theme.HelpText.Render("  (araç yok)")
	}
	b.WriteString(a.renderAccordion(accordion{title: "Araçlar", body: toolsBody, open: true}))
	b.WriteString("\n\n")
	// Skills (collapsed, shimmer if lazy).
	skillsBody := "(yapılandırılmadı)"
	if p.Lazy {
		// Render a short shimmer placeholder.
		shim := a.shimmerSkeleton(width - 12, 3)
		skillsBody = shim
	} else if len(p.Skills) > 0 {
		skillsBody = a.theme.HelpText.Render("  " + truncLine(p.Skills, width-12))
	}
	count := len(p.Skills)
	b.WriteString(a.renderAccordion(accordion{title: "Yetenekler", count: &count, body: skillsBody, open: false}))
	b.WriteString("\n\n")
	// System prompt (collapsed, show first line).
	spBody := ""
	if p.SystemPrompt != "" {
		first := strings.SplitN(p.SystemPrompt, "\n", 2)[0]
		spBody = a.theme.HelpText.Render("  " + truncateToWidth(first, width-12))
	} else {
		spBody = a.theme.HelpText.Render("  (yok)")
	}
	b.WriteString(a.renderAccordion(accordion{title: "Sistem Promptu", body: spBody, open: false}))
	b.WriteString("\n\n")
	// MCP servers (collapsed).
	mcpBody := ""
	if p.Lazy {
		mcpBody = a.shimmerSkeleton(width-12, 2)
	} else if len(p.MCPServers) > 0 {
		mcpBody = a.theme.HelpText.Render("  " + truncLine(p.MCPServers, width-12))
	} else {
		mcpBody = a.theme.HelpText.Render("  (yapılandırılmadı)")
	}
	count = len(p.MCPServers)
	b.WriteString(a.renderAccordion(accordion{title: "MCP Sunucuları", count: &count, body: mcpBody, open: false}))
	return b.String()
}

// shimmerSkeleton renders a fake "loading" line of width cells with
// dimmed-dots so the user sees the section is loading without a
// blocking "Loading..." text. Real Hermes uses the shimmer sweep
// from loaders.tsx; the Atlas port emits a static placeholder because
// Atlas doesn't yet animate shimmers per-section.
func (a *App) shimmerSkeleton(width, rows int) string {
	if width < 4 {
		width = 4
	}
	if rows < 1 {
		rows = 1
	}
	var b strings.Builder
	for r := 0; r < rows; r++ {
		if r > 0 {
			b.WriteString("\n")
		}
		b.WriteString(a.theme.HelpText.Render("  " + strings.Repeat("·", width-4)))
	}
	return b.String()
}

// buildSessionPanel gathers the panel data from the current App state.
// Called once at startup (or whenever the panel is re-shown via /welcome).
func (a *App) buildSessionPanel() SessionPanel {
	cwd, _ := os.Getwd()
	return SessionPanel{
		Info: SessionInfo{
			Provider: a.agent.ProviderName(),
			Model:    a.agent.CurrentModel(),
			CWD:      cwd,
			Branch:   a.gitBranch,
			StartedAt: time.Now(),
		},
		Tools:        a.agent.ToolNames(),
		SystemPrompt: a.systemPromptText,
		Lazy:         false,
	}
}
