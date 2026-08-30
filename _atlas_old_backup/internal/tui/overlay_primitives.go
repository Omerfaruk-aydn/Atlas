package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// clampOverlayWidth implements Hermes's clampOverlayWidth: honors the
// caller's hard cap absolutely, only enforces the floor when the cap
// allows it. The result is always in [min, max] when max >= min.
func clampOverlayWidth(preferred, maxWidth int, min ...int) int {
	floor := 24
	if len(min) > 0 {
		floor = min[0]
	}
	w := preferred
	if maxWidth > 0 && w > maxWidth {
		w = maxWidth
	}
	if w < floor {
		if maxWidth >= floor {
			w = floor
		} else {
			w = maxWidth
		}
	}
	if w < 1 {
		w = 1
	}
	return w
}

// scrollbarColors returns the (thumb, track) lipgloss.Styles for a
// scrollbar in the given hover/grab state. The track is a blend of
// border (or muted) toward completionBg at a 25% mix on idle, 55% on
// hover; the thumb is accent when grabbed/hovered else primary.
//
// Hermes's exact algorithm: track = mix(border, completionCurrentBg, 0.55)
// when hovered, else mix(muted, completionCurrentBg, 0.25). Atlas
// approximates with the Border color blended toward the surfaceBg.
func (a *App) scrollbarColors(hovered, grabbed bool) (thumb, track lipgloss.Style) {
	thumbColor := a.theme.Primary
	if hovered || grabbed {
		thumbColor = a.theme.Accent
	}
	trackMix := Mix(string(a.theme.Border.Dark), string(a.theme.SurfaceBg.Dark), 0.25)
	if hovered {
		trackMix = Mix(string(a.theme.Border.Dark), string(a.theme.SurfaceBg.Dark), 0.55)
	}
	thumb = lipgloss.NewStyle().Foreground(thumbColor)
	track = lipgloss.NewStyle().Foreground(lipgloss.Color(trackMix))
	return thumb, track
}

// useMenu is the canonical arrow + Enter + number-quick-pick menu
// dispatch. It's stateless: callers feed a key, the current selection,
// and the options list; the function returns the new selection or a
// "confirm" signal. Kept as a pure function (no Bubbletea imports) so
// the same logic backs every menu in the app — pickers, approval, etc.
//
// Returns:
//   - kind: "move" | "choose" | "noop" | "cancel"
//   - idx: the new selection index (for "move" / "choose")
func useMenu(key string, sel int, optsLen int) (kind string, idx int) {
	if optsLen <= 0 {
		return "noop", 0
	}
	switch key {
	case "up", "k":
		next := sel - 1
		if next < 0 {
			next = optsLen - 1
		}
		return "move", next
	case "down", "j", "tab":
		next := sel + 1
		if next >= optsLen {
			next = 0
		}
		return "move", next
	case "enter":
		return "choose", sel
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(key[0] - '1')
		if idx < 0 || idx >= optsLen {
			return "noop", sel
		}
		return "choose", idx
	case "esc", "ctrl+c":
		return "cancel", sel
	}
	return "noop", sel
}

// truncLine joins a comma-separated list of items into a single line
// that fits within maxWidth, appending ", …+N" when the budget runs out
// before every item is shown. Matches Hermes's truncLine in branding.tsx.
func truncLine(items []string, maxWidth int) string {
	if len(items) == 0 {
		return ""
	}
	if maxWidth < 8 {
		maxWidth = 8
	}
	const sep = ", "
	var b strings.Builder
	for i, it := range items {
		chunk := it
		if i > 0 {
			chunk = sep + it
		}
		// Tentatively add this item; if we exceed, fall back to "+N".
		candidate := b.String() + chunk
		if lipgloss.Width(candidate) > maxWidth {
			omitted := len(items) - i
			if i == 0 {
				// Single item larger than budget: truncate.
				return truncateToWidth(it, maxWidth)
			}
			return strings.TrimRight(b.String(), sep) + sep + "…+" + itoa(omitted)
		}
		b.WriteString(chunk)
	}
	return b.String()
}

// itoa is a tiny local int-to-string helper to avoid pulling in
// strconv for a one-liner.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// menuRow is a single row in a generic useMenu-backed list, with
// label/description/disabled state.
type menuRow struct {
	Label    string
	Desc     string
	Disabled bool
	OnChoose func()
}

// renderMenuList paints a useMenu-shaped list at the given selection.
// Used by the session switcher, model picker, approval, and any
// future overlay that needs arrow+number-key navigation.
func (a *App) renderMenuList(rows []menuRow, sel int, width int) string {
	if width < 8 {
		width = 8
	}
	// Compute label column width.
	labelW := 0
	for _, r := range rows {
		if w := len(r.Label); w > labelW {
			labelW = w
		}
	}
	labelW += 2

	var b strings.Builder
	for i, r := range rows {
		prefix := "  "
		style := a.theme.HelpText
		if i == sel {
			prefix = "▸ "
			style = a.theme.UserLabel
		}
		if r.Disabled {
			style = a.theme.HelpText
		}
		line := prefix + padRight(r.Label, labelW) + r.Desc
		if i == sel {
			b.WriteString(a.theme.SelectedBgBackground(style.Render(line)))
		} else {
			b.WriteString(style.Render(line))
		}
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// padRight pads s with spaces to width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
