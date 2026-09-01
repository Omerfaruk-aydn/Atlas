package logo

import (
	"fmt"
	"image/color"
	"strings"
	"sync"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-style/v2"
	"github.com/lucasb-eyer/go-colorful"
)

// bannerText is the wordmark rendered by [RenderPixelBanner].
const bannerText = "ATLAS-AGENT"

// bannerRows is the height, in terminal rows, of every glyph in the
// wordmark font.
const bannerRows = 6

// bannerGlyphs holds the wordmark letterforms in the "ANSI Shadow" style:
// solid blocks for the face with box-drawing characters tracing the bevel,
// which is what gives the art its extruded, three-dimensional edge. Every
// row of a glyph is the same width so glyphs concatenate cleanly.
var bannerGlyphs = map[rune][bannerRows]string{
	'A': {
		" █████╗ ",
		"██╔══██╗",
		"███████║",
		"██╔══██║",
		"██║  ██║",
		"╚═╝  ╚═╝",
	},
	'T': {
		"████████╗",
		"╚══██╔══╝",
		"   ██║   ",
		"   ██║   ",
		"   ██║   ",
		"   ╚═╝   ",
	},
	'L': {
		"██╗     ",
		"██║     ",
		"██║     ",
		"██║     ",
		"███████╗",
		"╚══════╝",
	},
	'S': {
		"███████╗",
		"██╔════╝",
		"███████╗",
		"╚════██║",
		"███████║",
		"╚══════╝",
	},
	'G': {
		" ██████╗ ",
		"██╔════╝ ",
		"██║  ███╗",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
	'E': {
		"███████╗",
		"██╔════╝",
		"█████╗  ",
		"██╔══╝  ",
		"███████╗",
		"╚══════╝",
	},
	'N': {
		"███╗   ██╗",
		"████╗  ██║",
		"██╔██╗ ██║",
		"██║╚██╗██║",
		"██║ ╚████║",
		"╚═╝  ╚═══╝",
	},
	'-': {
		"      ",
		"      ",
		"█████╗",
		"╚════╝",
		"      ",
		"      ",
	},
	' ': {
		"   ",
		"   ",
		"   ",
		"   ",
		"   ",
		"   ",
	},
}

// Banner colors, applied as per-row bands rather than a smooth ramp: the
// face reads as bright gold at the top and settles into a burnt orange
// along the baseline.
var (
	bannerGold   = color.RGBA{R: 0xFF, G: 0xD4, B: 0x26, A: 0xFF}
	bannerAmber  = color.RGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xFF}
	bannerBronze = color.RGBA{R: 0xC8, G: 0x7F, B: 0x32, A: 0xFF}
)

// bannerRowColors maps each glyph row to its band color.
var bannerRowColors = [bannerRows]color.Color{
	bannerGold, bannerGold,
	bannerAmber, bannerAmber,
	bannerBronze, bannerBronze,
}

// bannerLines composes the wordmark into one string per row.
func bannerLines() [bannerRows]string {
	var rows [bannerRows]string
	var b [bannerRows]strings.Builder
	for _, r := range bannerText {
		glyph, found := bannerGlyphs[r]
		if !found {
			glyph = bannerGlyphs[' ']
		}
		for i := range bannerRows {
			b[i].WriteString(glyph[i])
		}
	}
	for i := range bannerRows {
		rows[i] = b[i].String()
	}
	return rows
}

// BannerSize returns the terminal columns and rows [RenderPixelBanner]
// needs at the given available width, and whether it fits at all. Callers
// that own the layout use this to reserve header space before rendering.
func BannerSize(availWidth int) (cols, rows int, ok bool) {
	cols = bannerCols()
	if cols == 0 || cols > availWidth {
		return 0, 0, false
	}
	return cols, bannerRows, true
}

// bannerCols is the wordmark's width in terminal columns. The wordmark text
// is fixed, so this is a constant for any given build.
func bannerCols() int {
	cols := 0
	for _, l := range bannerLines() {
		cols = max(cols, len([]rune(l)))
	}
	return cols
}

// RenderPixelBanner draws the wordmark sized to the given available width.
// It returns "" when the wordmark cannot fit, in which case callers should
// fall back to the compact lettering.
func RenderPixelBanner(availWidth int) string {
	if _, _, ok := BannerSize(availWidth); !ok {
		return ""
	}

	lines := bannerLines()
	var out strings.Builder
	for i, l := range lines {
		out.WriteString(lipgloss.NewStyle().
			Foreground(bannerRowColors[i]).
			Render(strings.TrimRight(l, " ")))
		if i < bannerRows-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// rainbowSaturation and rainbowValue keep the sweep vivid without the harsh
// clipping of fully saturated primaries.
const (
	rainbowSaturation = 0.85
	rainbowValue      = 1.0
	// rowHueSkew shifts each successive row along the ramp so the sweep
	// travels diagonally rather than as a flat vertical bar.
	rowHueSkew = 2
)

var (
	rampOnce  sync.Once
	rampCache []string
)

// rainbowRamp returns one prebuilt ANSI foreground escape per ramp slot,
// sweeping a full hue circle so the animation loops without a visible seam.
// Escapes are formatted once and reused every frame: at 60fps the render
// path runs hundreds of times a second, so per-cell color formatting would
// dominate it. The ramp is exactly as wide as the wordmark, which makes the
// wordmark's sweep period equal to its own width, and lets every other
// rainbow surface share the same hue sequence.
func rainbowRamp() []string {
	rampOnce.Do(func() {
		size := bannerCols()
		rampCache = make([]string, size)
		for i := range rampCache {
			hue := float64(i) / float64(size) * 360
			r, g, b := colorful.Hsv(hue, rainbowSaturation, rainbowValue).RGB255()
			rampCache[i] = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
		}
	})
	return rampCache
}

// rainbowAt returns the ANSI foreground escape for a cell, given its column
// and row within a surface plus a scroll offset. The row skew is what makes
// the sweep travel diagonally.
func rainbowAt(ramp []string, x, y, frame int) string {
	n := len(ramp)
	return ramp[((x+y*rowHueSkew+frame)%n+n)%n]
}

// RenderAnimatedBanner draws the wordmark with a rainbow sweep scrolled by
// frame, which callers advance once per animation tick. It returns "" when
// the wordmark cannot fit at the given width.
func RenderAnimatedBanner(availWidth, frame int) string {
	if _, _, ok := BannerSize(availWidth); !ok {
		return ""
	}
	ramp := rainbowRamp()
	if len(ramp) == 0 {
		return RenderPixelBanner(availWidth)
	}

	lines := bannerLines()
	var out strings.Builder
	for i, l := range lines {
		for x, ch := range []rune(strings.TrimRight(l, " ")) {
			if ch == ' ' {
				// Unstyled gaps: no escape needed, and skipping them keeps
				// the per-frame output meaningfully smaller.
				out.WriteRune(' ')
				continue
			}
			out.WriteString(rainbowAt(ramp, x, i, frame))
			out.WriteRune(ch)
		}
		out.WriteString("\x1b[0m")
		if i < bannerRows-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
