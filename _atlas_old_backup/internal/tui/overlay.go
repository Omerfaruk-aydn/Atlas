package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// OverlayPlacement is the 9-zone grid position for an overlay. Hermes
// supports the full 3x3 (corners, edges, center); for the most part
// Atlas uses center for modals and bottom for floating popovers (the
// completion menu sits at the bottom of the chat pane).
type OverlayPlacement int

const (
	OverlayTopLeft OverlayPlacement = iota
	OverlayTop
	OverlayTopRight
	OverlayLeft
	OverlayCenter
	OverlayRight
	OverlayBottomLeft
	OverlayBottom
	OverlayBottomRight
)

// overlayBox is one rendered overlay + its desired dimensions. The
// placement grid + scrim option are used to compose it into the parent
// frame in App.View. Pure data — no render pass here.
type overlayBox struct {
	content string
	width   int
	height  int
	placement OverlayPlacement
	scrim   bool // when true, paint the parent with a dim background fill behind
}

// renderOverlay paints content into a sub-frame of the given width /
// height, placed according to placement. base is the underlying frame
// (typically the chat viewport text). The scrim, when enabled, paints
// the full parent with blank rows at background color so the overlay
// reads as opaque against whatever was underneath — Ink can't paint a
// true alpha background, so we approximate with bg-colored space rows.
func renderOverlay(base, content string, width, height int, placement OverlayPlacement, scrim bool) string {
	if content == "" {
		return base
	}
	// Build a buffer of `height` rows, each of `width` cells. The
	// overlay is positioned by computing the row/col offset of the top-
	// left corner; cells outside the overlay rectangle are left as the
	// scrim (or unchanged if scrim is false).
	rows := strings.Split(base, "\n")
	// Pad to height.
	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}
	// Compute overlay top-left offset.
	top, left := overlayOffset(placement, width, height)
	// Build the overlay's own row layout.
	oLines := strings.Split(content, "\n")
	oH := len(oLines)
	if top+oH > height {
		oH = height - top
		if oH < 0 {
			oH = 0
		}
	}
	for i := 0; i < oH; i++ {
		row := rows[top+i]
		// Pad row to `width` so we can splice.
		cur := lipgloss.Width(row)
		if cur < width {
			row += strings.Repeat(" ", width-cur)
		}
		// Splice the overlay line into the row.
		overlay := oLines[i]
		overlayW := lipgloss.Width(overlay)
		if overlayW > width-left {
			overlay = truncateToWidth(overlay, width-left)
			overlayW = lipgloss.Width(overlay)
		}
		// Apply scrim: replace the row with bg-colored space, then
		// paint overlay on top.
		if scrim {
			row = strings.Repeat(" ", width)
		}
		// Splice by visible column. Easier: just concatenate the three
		// segments, padding the left and right as needed.
		leftPart := ""
		if left > 0 {
			leftPart = truncateToWidth(row, left)
			if lipgloss.Width(leftPart) < left {
				leftPart += strings.Repeat(" ", left-lipgloss.Width(leftPart))
			}
		}
		rightStart := left + overlayW
		rightPart := ""
		if rightStart < width {
			rightPart = truncateFromColumn(row, rightStart)
		}
		rows[top+i] = leftPart + overlay + rightPart
	}
	return strings.Join(rows, "\n")
}

// overlayOffset returns the (top, left) of the top-left corner of an
// overlay for the given placement, in a frame of (frameW, frameH) cells.
// Centered placements get the overlay centered; corners are at 0/0 with
// a 1-cell margin; edges are pinned to one side.
func overlayOffset(p OverlayPlacement, frameW, frameH int) (int, int) {
	// Default content size — assume full frame for offset math; the
	// actual overlay size is communicated via overlayBox.width/height.
	contentW := frameW / 2
	contentH := frameH / 3
	switch p {
	case OverlayTopLeft:
		return 1, 1
	case OverlayTop:
		return 1, (frameW - contentW) / 2
	case OverlayTopRight:
		return 1, frameW - contentW - 1
	case OverlayLeft:
		return (frameH - contentH) / 2, 1
	case OverlayCenter:
		return (frameH - contentH) / 2, (frameW - contentW) / 2
	case OverlayRight:
		return (frameH - contentH) / 2, frameW - contentW - 1
	case OverlayBottomLeft:
		return frameH - contentH - 1, 1
	case OverlayBottom:
		return frameH - contentH - 1, (frameW - contentW) / 2
	case OverlayBottomRight:
		return frameH - contentH - 1, frameW - contentW - 1
	}
	return 0, 0
}

// truncateToWidth returns the prefix of s that fits within w display
// columns, appending an ellipsis when truncation happens. ANSI-aware
// (uses lipgloss.Width).
func truncateToWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if col+rw+1 > w {
			b.WriteString("…")
			return b.String()
		}
		b.WriteRune(r)
		col += rw
	}
	return b.String()
}

// truncateFromColumn returns the substring of s starting at visible
// column `col`. Used to extract a row's right-side suffix after the
// overlay is spliced in.
func truncateFromColumn(s string, col int) string {
	if col <= 0 {
		return s
	}
	cur := 0
	for i, r := range s {
		if cur >= col {
			return s[i:]
		}
		cur += lipgloss.Width(string(r))
	}
	return ""
}

// dialogBox wraps a title + body in the Hermes Dialog primitive: a
// bordered round box with the title centered along the top border and
// an optional hint footer at the bottom.
type dialogBox struct {
	title       string
	body        string
	hint        string
	width       int
	border      lipgloss.Style
	centerTitle bool
	borderSet   bool
}

// renderDialog returns the bordered round-box frame.
func (a *App) renderDialog(d dialogBox) string {
	width := d.width
	if width < 10 {
		width = 10
	}
	border := d.border
	if !d.borderSet {
		border = a.theme.InputBox
	}
	// Title row: if there's a title, render it as the first body line.
	var body string
	if d.title != "" {
		title := a.theme.Title.Render(d.title)
		if d.centerTitle {
			title = lipgloss.NewStyle().Width(width - 2).Align(lipgloss.Center).Render(title)
		}
		body = title + "\n" + d.body
	} else {
		body = d.body
	}
	if d.hint != "" {
		body += "\n" + a.theme.HelpText.Render(d.hint)
	}
	return border.Width(width).Render(body)
}
