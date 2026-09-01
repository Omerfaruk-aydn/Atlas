// Package logo renders an ATLAS-AGENT wordmark in a stylized way.
package logo

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/maincodss/atlas-agent/internal/deps/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/ui/styles"
	"github.com/maincodss/atlas-agent/internal/deps/cb/x/ansi"
)

// letterform represents a letterform. It can be stretched horizontally by
// a given amount via the boolean argument.
type letterform func(bool) string

// Opts are the options for rendering the ATLAS-AGENT title art.
type Opts struct {
	TitleColorA  color.Color // left gradient ramp point
	TitleColorB  color.Color // right gradient ramp point
	AtlasColor   color.Color // Atlas™ text color
	VersionColor color.Color // version text color
	Width        int         // width of the rendered logo, used for truncation
	Frame        int         // animation frame for the wide banner's rainbow sweep
	Hyper        bool        // whether it is ATLAS-AGENT or Hyper ATLAS-AGENT

	// When true, stretch a random letterform on each render. Has no effect in
	// compact mode. Mainly for testing. In production you will want to cache
	// the stretched letterform to keep the logo from jittering on resize.
	Unstable bool
}

// Render renders the ATLAS-AGENT logo. Set the argument to true to render the narrow
// version, intended for use in a sidebar.
//
// The compact argument determines whether it renders compact for the sidebar
// or wider for the main pane.
func Render(base lipgloss.Style, version string, compact bool, o Opts) string {
	// Wide mode gets the pixel-art wordmark when it fits; otherwise fall
	// through to the compact lettering below.
	if !compact {
		if banner := RenderAnimatedBanner(o.Width, o.Frame); banner != "" {
			return banner
		}
	}

	Atlas := "Atlas™"
	if !o.Hyper {
		Atlas = " " + Atlas
	}

	fg := func(c color.Color, s string) string {
		return lipgloss.NewStyle().Foreground(c).Render(s)
	}

	// Title. Pick the widest lettering that fits; if none does, fall back to
	// plain text rather than overflowing the caller's frame.
	wordmark := fitWordmark(o.Hyper, o.Width)
	if wordmark == "" {
		return plainWordmark(base, o)
	}
	wordmarkWidth := lipgloss.Width(wordmark)
	b := new(strings.Builder)
	for r := range strings.SplitSeq(wordmark, "\n") {
		fmt.Fprintln(b, styles.ApplyForegroundGrad(base, r, o.TitleColorA, o.TitleColorB))
	}
	wordmark = b.String()

	// Atlas and version. Both are clipped against the wordmark's width so the
	// meta row can never be the line that overflows: the wordmark is already
	// known to fit, and the row is laid out to span exactly its width.
	const metaRowGap = 1
	Atlas = ansi.Truncate(Atlas, wordmarkWidth, "…")
	if maxVersionWidth := wordmarkWidth - lipgloss.Width(Atlas) - metaRowGap; maxVersionWidth > 0 {
		version = ansi.Truncate(version, maxVersionWidth, "…")
	} else {
		version = ""
	}
	if o.Hyper && version != "" {
		version += " "
	}
	gap := max(0, wordmarkWidth-lipgloss.Width(Atlas)-lipgloss.Width(version))
	metaRow := fg(o.AtlasColor, Atlas) + strings.Repeat(" ", gap) + fg(o.VersionColor, version)

	// Join the meta row and big ATLAS-AGENT title.
	wordmark = strings.TrimSpace(metaRow + "\n" + wordmark)

	// Narrow version. If this is Hyper ATLAS-AGENT, this is also a stacked version.
	// Kept minimal on purpose: no decorative filler, just the wordmark.
	return wordmark
}

// compactWordmarks lists the compact lettering variants from widest to
// narrowest. [fitWordmark] takes the first that fits, so a narrow sidebar or
// exit banner sheds "-AGENT" before it gives up on the lettering altogether.
var compactWordmarks = [][]letterform{
	{LetterA, LetterT, LetterL, LetterA, LetterSAlt, LetterDash, LetterA, LetterG, LetterE, LetterN, LetterT},
	{LetterA, LetterT, LetterL, LetterA, LetterSAlt},
}

// hyperWordmark is the "HYPER" row stacked above the wordmark in Hyper mode.
var hyperWordmark = []letterform{LetterH, LetterYAlt, LetterP, LetterE, LetterR}

// fitWordmark renders the widest compact lettering that fits within limit,
// or the empty string when even the narrowest does not. A limit of zero or
// less means unbounded.
func fitWordmark(hyper bool, limit int) string {
	// -1 means no stretching; compact mode never stretches.
	const spacing, stretchIndex = 1, -1

	for _, letterforms := range compactWordmarks {
		word := renderWord(spacing, stretchIndex, letterforms...)
		if hyper {
			word = renderWord(spacing, stretchIndex, hyperWordmark...) + "\n" + word
		}
		if limit <= 0 || lipgloss.Width(word) <= limit {
			return word
		}
	}
	return ""
}

// plainWordmark is the last-resort logo for widths too narrow for any
// lettering: a single gradient line, truncated if even that overflows.
func plainWordmark(base lipgloss.Style, o Opts) string {
	name := "ATLAS-AGENT"
	if o.Hyper {
		name = "HYPER ATLAS-AGENT"
	}
	if o.Width > 0 {
		name = ansi.Truncate(name, o.Width, "…")
	}
	return styles.ApplyForegroundGrad(base, name, o.TitleColorA, o.TitleColorB)
}

// SmallRender renders a smaller version of the ATLAS-AGENT logo, suitable for
// smaller windows or sidebar usage.
func SmallRender(t *styles.Styles, width int, o Opts) string {
	name := "ATLAS-AGENT"
	if o.Hyper {
		name = "HYPER ATLAS-AGENT"
	}
	Atlas := "Atlas™"
	title := t.Logo.SmallAtlas.Render(Atlas)
	title = fmt.Sprintf("%s %s", title, styles.ApplyBoldForegroundGrad(t.Logo.GradCanvas, name, t.Logo.SmallGradFromColor, t.Logo.SmallGradToColor))
	return title
}
