package model

import (
	"image/color"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// highlightFadeDuration is how long a freshly-added sidebar row stays
// tinted with the accent color before settling to its resting color.
const highlightFadeDuration = 900 * time.Millisecond

// highlightTickInterval paces the redraws that animate the fade; fast
// enough to look smooth, short-lived enough not to matter for CPU usage.
const highlightTickInterval = 50 * time.Millisecond

// highlightTracker fades in newly-appeared items in a list-like sidebar
// section (e.g. a job that just started). It distinguishes "item existed
// before the tracker started watching" (no highlight, so the whole list
// doesn't flash on first load) from "item appeared after that" (highlighted).
type highlightTracker struct {
	seen        map[string]struct{}
	startedAt   map[string]time.Time
	initialized bool
}

// sync reconciles the tracker against the current set of item keys,
// starting a fade for any key that's new since the last call (except on
// the very first call, which just records the baseline). It reports
// whether any item is still mid-fade, so the caller knows whether to keep
// scheduling redraw ticks.
func (h *highlightTracker) sync(currentKeys map[string]struct{}) bool {
	if h.seen == nil {
		h.seen = make(map[string]struct{}, len(currentKeys))
		h.startedAt = make(map[string]time.Time, len(currentKeys))
	}
	now := time.Now()
	if !h.initialized {
		h.initialized = true
		for k := range currentKeys {
			h.seen[k] = struct{}{}
		}
		return false
	}
	for k := range currentKeys {
		if _, ok := h.seen[k]; !ok {
			h.seen[k] = struct{}{}
			h.startedAt[k] = now
		}
	}
	// Forget keys that disappeared, so if a key is reused later (e.g. a
	// job ID) it's treated as new again rather than skipped.
	for k := range h.seen {
		if _, ok := currentKeys[k]; !ok {
			delete(h.seen, k)
			delete(h.startedAt, k)
		}
	}
	active := false
	for k, t := range h.startedAt {
		if now.Sub(t) >= highlightFadeDuration {
			delete(h.startedAt, k)
			continue
		}
		active = true
	}
	return active
}

// progress returns 1 right when key started fading, decaying linearly to 0
// over highlightFadeDuration, and 0 once it's done or was never marked.
func (h *highlightTracker) progress(key string) float64 {
	t, ok := h.startedAt[key]
	if !ok {
		return 0
	}
	elapsed := time.Since(t)
	if elapsed >= highlightFadeDuration {
		return 0
	}
	return 1 - float64(elapsed)/float64(highlightFadeDuration)
}

// highlightTickMsg drives the sidebar redraw loop while a highlight fade is
// in progress.
type highlightTickMsg struct{}

func highlightTickCmd() tea.Cmd {
	return tea.Tick(highlightTickInterval, func(time.Time) tea.Msg {
		return highlightTickMsg{}
	})
}

// dialogDimTarget is how dark the backdrop gets behind an open dialog once
// the open transition settles (0 = unchanged, 1 = black).
const dialogDimTarget = 0.35

// dialogFadeTickMsg drives the redraw loop that plays a dialog's backdrop
// dim-in transition; see dialog.Overlay.OpenProgress.
type dialogFadeTickMsg struct{}

func dialogFadeTickCmd() tea.Cmd {
	return tea.Tick(highlightTickInterval, func(time.Time) tea.Msg {
		return dialogFadeTickMsg{}
	})
}

// dimScreen darkens every drawn cell's foreground and background color by
// amount (0 = unchanged, 1 = black); cells with no color set (terminal
// default) are left alone. Used to dim the backdrop behind an open dialog.
func dimScreen(scr uv.Screen, amount float64) {
	if amount <= 0 {
		return
	}
	bounds := scr.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cell := scr.CellAt(x, y)
			if cell == nil {
				continue
			}
			changed := false
			if cell.Style.Fg != nil {
				cell.Style.Fg = darken(cell.Style.Fg, amount)
				changed = true
			}
			if cell.Style.Bg != nil {
				cell.Style.Bg = darken(cell.Style.Bg, amount)
				changed = true
			}
			if changed {
				scr.SetCell(x, y, cell)
			}
		}
	}
}

// darken blends c toward black by amount (0..1) using plain linear RGB math
// (not perceptual blending) since this runs over every on-screen cell.
func darken(c color.Color, amount float64) color.Color {
	r, g, b, a := c.RGBA()
	factor := 1 - amount
	return color.RGBA64{
		R: uint16(float64(r) * factor),
		G: uint16(float64(g) * factor),
		B: uint16(float64(b) * factor),
		A: uint16(a),
	}
}
