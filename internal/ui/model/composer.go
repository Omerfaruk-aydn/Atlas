package model

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/permission"
	"github.com/maincodss/atlas-agent/internal/ui/logo"
	"github.com/maincodss/atlas-agent/internal/ui/styles"
)

// Composer frame geometry. The textarea sits inside a rounded border with one
// padding column on each side, so everything that maps screen coordinates to
// textarea coordinates — the cursor, mouse events, the layout height — has to
// account for the same offsets.
const (
	// composerPadX is the left border glyph plus its padding column.
	composerPadX = 2
	// composerPadY is the top border row.
	composerPadY = 1
	// composerChrome is the total width the frame takes from the textarea.
	composerChrome = composerPadX * 2
	// composerBorderRows is the top and bottom edge.
	composerBorderRows = 2
	// composerTitleLeadIn is how far along the top edge the mode label
	// starts, leaving one horizontal glyph before it.
	composerTitleLeadIn = 2
	// composerFrameDivisor halves the wordmark's 60fps tick so yolo mode's
	// rainbow border sweeps at 30fps. Dividing the existing chain rather
	// than starting a timer of its own keeps it in step with the landing
	// cards and costs nothing extra.
	composerFrameDivisor = 2
)

// composerRainbow reports whether the composer frame should sweep the
// wordmark's rainbow instead of a solid color. Yolo mode runs without
// permission prompts, so its frame is the loudest thing on screen on purpose.
// Bang mode overrides the permission mode in the prompt, and so here too.
func (m *UI) composerRainbow() bool {
	return !m.bangMode && m.permissionModeCached() == permission.ModeBypass
}

// composerAccent returns the label and color for the composer frame, which
// track the active permission mode. Every mode but manual borrows the color of
// its own prompt dots, so a theme that restyles those restyles the frame to
// match automatically. Blurred variants are used when the editor doesn't have
// focus.
func (m *UI) composerAccent(mode permission.PermissionMode, focused bool) (string, color.Color) {
	t := m.com.Styles
	pick := func(focusedStyle, blurredStyle lipgloss.Style) color.Color {
		if focused {
			return focusedStyle.GetForeground()
		}
		return blurredStyle.GetForeground()
	}

	if m.bangMode {
		return "shell", pick(t.Editor.PromptBangDotsFocused, t.Editor.PromptBangDotsBlurred)
	}
	switch mode {
	case permission.ModeBypass:
		return "yolo", pick(t.Editor.PromptYoloDotsFocused, t.Editor.PromptYoloDotsBlurred)
	case permission.ModePlan:
		return "plan", pick(t.Editor.PromptPlanDotsFocused, t.Editor.PromptPlanDotsBlurred)
	case permission.ModeAutoAcceptEdits:
		return "auto-accept", pick(t.Editor.PromptAutoAcceptDotsFocused, t.Editor.PromptAutoAcceptDotsBlurred)
	default:
		// Manual takes the theme's frame color rather than the prompt
		// green, which is deliberately near-invisible on the dots and
		// would leave the frame barely there.
		if focused {
			return "manual", t.Editor.ComposerAccent
		}
		return "manual", t.Editor.PromptNormalBlurred.GetForeground()
	}
}

// composerFrame wraps the textarea in a rounded border with the active
// permission mode inlaid in the top edge, matching the landing cards. Most
// modes get a solid color; yolo gets the wordmark's rainbow sweep, because a
// mode that skips every permission prompt should be impossible to forget you
// are in.
//
// The result is always exactly width columns wide and
// len(content lines)+composerBorderRows tall, because the layout, cursor, and
// mouse mapping are all computed from those numbers.
func (m *UI) composerFrame(content string, width int) string {
	label, accent := m.composerAccent(m.permissionModeCached(), m.textarea.Focused())

	inner := max(0, width-composerChrome)
	lines := strings.Split(content, "\n")

	// RainbowBox draws the same frame this function does — same chrome, same
	// title lead-in — so yolo mode reuses it rather than duplicating the
	// geometry that the cursor and mouse mapping depend on. Below the width
	// where it would give up and return the content bare, fall through to
	// the solid path, which always frames.
	if m.composerRainbow() && width > composerChrome {
		return logo.RainbowBox(label, content, width, len(lines), m.bannerFrame/composerFrameDivisor, 0)
	}

	border := lipgloss.NewStyle().Foreground(accent)

	var b strings.Builder

	// Top edge with the mode label inlaid: ┌─ manual ─────┐
	// The cursor is tracked explicitly so the trailing rule always runs
	// exactly up to the corner, whether or not the label fits.
	top := styles.BoxTopLeft
	if lw := lipgloss.Width(label); lw > 0 && lw <= inner-composerTitleLeadIn {
		top += strings.Repeat(styles.BoxHorizontal, composerTitleLeadIn-1) + " " + label + " "
	}
	if fill := width - 1 - lipgloss.Width(top); fill > 0 {
		top += strings.Repeat(styles.BoxHorizontal, fill)
	}
	b.WriteString(border.Render(top + styles.BoxTopRight))
	b.WriteByte('\n')

	// Interior rows. Only the border glyphs are styled; the textarea's own
	// output passes through untouched so its prompt, selection, and syntax
	// colors survive.
	edge := border.Render(styles.BoxVertical)
	for _, line := range lines {
		b.WriteString(edge)
		b.WriteString(" ")
		b.WriteString(padTo(line, inner))
		b.WriteString(" ")
		b.WriteString(edge)
		b.WriteByte('\n')
	}

	b.WriteString(border.Render(
		styles.BoxBottomLeft +
			strings.Repeat(styles.BoxHorizontal, max(0, width-2)) +
			styles.BoxBottomRight,
	))
	return b.String()
}

// padTo pads a styled line out to width display columns, or truncates it when
// it overflows. Width is measured with lipgloss so ANSI styling in the line
// doesn't push the right border out of alignment.
func padTo(line string, width int) string {
	switch gap := width - lipgloss.Width(line); {
	case gap > 0:
		return line + strings.Repeat(" ", gap)
	case gap < 0:
		truncated := lipgloss.NewStyle().MaxWidth(width).Render(line)
		if pad := width - lipgloss.Width(truncated); pad > 0 {
			truncated += strings.Repeat(" ", pad)
		}
		return truncated
	default:
		return line
	}
}
