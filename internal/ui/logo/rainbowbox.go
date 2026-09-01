package logo

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/ui/styles"
)

const (
	boxHorizontal = styles.BoxHorizontal
	boxVertical   = styles.BoxVertical

	// ansiReset closes a run of rainbow-colored border glyphs so the box's
	// contents keep their own theme colors.
	ansiReset = "\x1b[0m"

	// boxChrome is the horizontal space a border consumes: two border
	// glyphs plus one padding column on each side.
	boxChrome = 4

	// titleLeadIn is how far along the top edge the title starts, leaving a
	// single horizontal glyph before it.
	titleLeadIn = 2
)

// RainbowBox draws content inside a rounded border whose glyphs carry the
// same rainbow sweep as the wordmark, scrolled by frame. Only the border is
// colored: the title and the content keep whatever styling the caller gave
// them, so the box frames the text rather than competing with it.
//
// width is the box's total outer width. xOffset is the box's own starting
// column within the row it belongs to, so a row of boxes reads as one
// continuous wave instead of three restarting ones. When height is positive
// the content is padded or truncated to exactly that many interior rows,
// which is what keeps side-by-side boxes the same height.
func RainbowBox(title, content string, width, height, frame, xOffset int) string {
	if width < boxChrome+1 {
		return content
	}
	ramp := rainbowRamp()
	if len(ramp) == 0 {
		return content
	}

	// paint returns a border glyph carrying the sweep color for its cell.
	paint := func(glyph string, x, y int) string {
		return rainbowAt(ramp, x+xOffset, y, frame) + glyph + ansiReset
	}
	// rule paints a run of horizontal glyphs from x through x+n-1.
	rule := func(b *strings.Builder, x, n, y int) {
		for i := range n {
			b.WriteString(paint(boxHorizontal, x+i, y))
		}
	}

	inner := width - boxChrome
	lines := padLines(content, inner, height)

	var out strings.Builder

	// Top edge, with the title inlaid: ╭─ Title ─────╮
	// The cursor is tracked explicitly so the trailing rule always runs
	// exactly up to the corner, whether or not a title was written.
	x := 0
	out.WriteString(paint(styles.BoxTopLeft, x, 0))
	x++
	if titleW := lipgloss.Width(title); titleW > 0 && titleW <= inner-titleLeadIn {
		rule(&out, x, titleLeadIn-1, 0)
		x += titleLeadIn - 1
		out.WriteString(" " + title + " ")
		x += titleW + 2
	}
	rule(&out, x, width-1-x, 0)
	out.WriteString(paint(styles.BoxTopRight, width-1, 0))
	out.WriteByte('\n')

	// Interior rows. The row index used for the sweep includes the top
	// edge, so the diagonal runs unbroken from the top border down the
	// sides and into the bottom.
	for i, line := range lines {
		y := i + 1
		out.WriteString(paint(boxVertical, 0, y))
		out.WriteString(" ")
		out.WriteString(line)
		out.WriteString(" ")
		out.WriteString(paint(boxVertical, width-1, y))
		out.WriteByte('\n')
	}

	// Bottom edge.
	y := len(lines) + 1
	out.WriteString(paint(styles.BoxBottomLeft, 0, y))
	rule(&out, 1, width-2, y)
	out.WriteString(paint(styles.BoxBottomRight, width-1, y))

	return out.String()
}

// padLines splits content into display lines, pads each to inner columns,
// and — when height is positive — pads or truncates the line count to match.
// Padding is measured with lipgloss.Width so ANSI styling in the content
// doesn't throw the right border out of alignment.
func padLines(content string, inner, height int) []string {
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	if height > 0 {
		for len(lines) < height {
			lines = append(lines, "")
		}
		lines = lines[:height]
	}
	for i, l := range lines {
		if gap := inner - lipgloss.Width(l); gap > 0 {
			lines[i] = l + strings.Repeat(" ", gap)
		} else if gap < 0 {
			lines[i] = lipgloss.NewStyle().MaxWidth(inner).Render(l)
			if pad := inner - lipgloss.Width(lines[i]); pad > 0 {
				lines[i] += strings.Repeat(" ", pad)
			}
		}
	}
	return lines
}
