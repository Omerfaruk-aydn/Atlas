package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// SessionEntry represents one row in the session switcher overlay.
// Each row has a status glyph, a session ID, a model label, a start
// time, and a one-line preview. The switcher shows both live sessions
// (running) and resumable history (from disk).
type SessionEntry struct {
	ID         string
	Title      string
	Model      string
	CWD        string
	StartedAt  time.Time
	Status     SessionStatus
	IsLive     bool
	Preview    string
}

// SessionStatus is the per-session indicator. The status glyph + color
// come from this enum; the live-session poll re-anchors selection by
// ID (NOT flat index) so the cursor doesn't desync when the list grows
// or shrinks in the background.
type SessionStatus int

const (
	SessionStatusIdle SessionStatus = iota
	SessionStatusStarting
	SessionStatusWorking
	SessionStatusWaiting
)

func (s SessionStatus) glyph() string {
	switch s {
	case SessionStatusIdle:
		return "✓"
	case SessionStatusStarting:
		return "…"
	case SessionStatusWaiting:
		return "?"
	case SessionStatusWorking:
		return "▶"
	}
	return "·"
}

func (s SessionStatus) style(a *App) lipgloss.Style {
	switch s {
	case SessionStatusIdle:
		return lipgloss.NewStyle().Foreground(a.theme.Success)
	case SessionStatusWorking:
		return a.theme.UserLabel
	case SessionStatusWaiting:
		return a.theme.HelpText
	default:
		return a.theme.HelpText
	}
}

// SessionSwitcherState is the full overlay state. Sel is the cursor
// position in the merged live+history list; ArmDelete is the second-
// press-of-d counter for two-press delete confirmation.
type SessionSwitcherState struct {
	Live      []SessionEntry
	History   []SessionEntry
	Sel       int
	Visible   int // window size for windowOffset
	Filter    string
	ArmDelete bool
	Width     int
}

// mergedRows returns the rendered row list: a pinned "+new" row at
// the top, then live sessions, then resumable history. The "+new" row
// is always at flat index 0 regardless of how the other lists grow.
func (s SessionSwitcherState) mergedRows() []SessionRow {
	out := []SessionRow{{Kind: SessionRowNew, Title: "+ yeni oturum"}}
	for _, e := range s.Live {
		out = append(out, SessionRow{Kind: SessionRowLive, Entry: e, Title: e.Title})
	}
	for _, e := range s.History {
		out = append(out, SessionRow{Kind: SessionRowHistory, Entry: e, Title: e.Title})
	}
	return out
}

// SessionRowKind tags a row in the merged list. The picker dispatcher
// uses it to know which list the cursor is in.
type SessionRowKind int

const (
	SessionRowNew SessionRowKind = iota
	SessionRowLive
	SessionRowHistory
)

type SessionRow struct {
	Kind  SessionRowKind
	Entry SessionEntry
	Title string // pre-extracted title for the "+new" pseudo-row
}

// renderSessionSwitcher paints the overlay. Title is centered, columns
// are width-fixed (2/11/11/18) with a flex-grow title column, all
// wrap="truncate-end". Two-press delete confirmation is shown inline
// at the bottom of the active row when armed.
func (a *App) renderSessionSwitcher(s SessionSwitcherState) string {
	width := s.Width
	if width < 64 {
		width = 64
	}
	if width > 128 {
		width = 128
	}
	rows := s.mergedRows()
	if s.Sel < 0 {
		s.Sel = 0
	}
	if s.Sel >= len(rows) {
		s.Sel = len(rows) - 1
	}
	off := windowOffset(s.Sel, s.Visible, len(rows))
	end := off + s.Visible
	if end > len(rows) {
		end = len(rows)
	}
	visible := rows[off:end]

	// Column widths. Fixed 2/11/11/18 cells for status, model, time,
	// and "ctrl-x" hint; the rest goes to the (truncated) title.
	statusW := 2
	modelW := 11
	timeW := 11
	actionW := 18
	titleW := width - statusW - modelW - timeW - actionW - 4
	if titleW < 16 {
		titleW = 16
	}

	// Header.
	var b strings.Builder
	b.WriteString(a.theme.Title.Render("Oturum değiştir"))
	b.WriteString("\n")
	b.WriteString(a.theme.HelpText.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n")
	// Column header row.
	hdr := padRight("", statusW) +
		padRight("model", modelW) +
		padRight("zaman", timeW) +
		padRight("eylem", actionW) +
		"başlık"
	b.WriteString(a.theme.HelpText.Render(hdr))
	b.WriteString("\n")
	// Rows.
	for i, row := range visible {
		absIdx := off + i
		cursor := "  "
		style := a.theme.HelpText
		if absIdx == s.Sel {
			cursor = "▸ "
			style = a.theme.UserLabel
		}
		var line string
		switch row.Kind {
		case SessionRowNew:
			line = cursor + padRight("+", statusW) +
				padRight("", modelW) +
				padRight("", timeW) +
				padRight("Ctrl+N", actionW) +
				"yeni oturum"
		case SessionRowLive, SessionRowHistory:
			e := row.Entry
			glyph := e.Status.glyph()
			age := fmtDuration(time.Since(e.StartedAt))
			model := truncateToWidth(e.Model, modelW)
			model = padRight(model, modelW)
			timeStr := padRight(age, timeW)
			action := ""
			switch row.Kind {
			case SessionRowLive:
				action = "Ctrl+D kapat"
			default:
				if s.ArmDelete && absIdx == s.Sel {
					action = "d · sil?"
				} else {
					action = "d sil"
				}
			}
			action = padRight(action, actionW)
			title := truncateToWidth(e.Title, titleW)
			if title == "" {
				title = truncateToWidth(e.ID, titleW)
			}
			line = cursor + e.Status.style(a).Render(padRight(glyph, statusW)) +
				model + timeStr + action + title
		}
		if absIdx == s.Sel {
			b.WriteString(a.theme.SelectedBgBackground(style.Render(line)))
		} else {
			b.WriteString(style.Render(line))
		}
		b.WriteString("\n")
	}
	// Footer hint.
	b.WriteString(a.theme.HelpText.Render(strings.Repeat("─", width-2)))
	b.WriteString("\n")
	b.WriteString(a.theme.HelpText.Render(
		"↑/↓ seç · Enter geç · Ctrl+N yeni · Ctrl+R yenile · Ctrl+D kapat · d sil · Esc kapat"))
	return a.theme.InputBox.Width(width).Render(b.String())
}

// sessionSwitcherKeyAction dispatches one key event. Returns
// (nextState, action) where action is "new" | "refresh" | "close-live" |
// "arm-delete" | "delete" | "select" | "move" | "filter" | "cancel" | "noop".
// Identity-preserving re-anchor: when the live list changes
// (background poll), the App re-resolves s.Sel by Entry.ID rather
// than flat index.
func sessionSwitcherKeyAction(s SessionSwitcherState, key string) (SessionSwitcherState, string) {
	next := s
	rows := s.mergedRows()
	if len(rows) == 0 {
		return next, "noop"
	}
	switch key {
	case "up", "k":
		next.Sel = (next.Sel - 1 + len(rows)) % len(rows)
		next.ArmDelete = false
		return next, "move"
	case "down", "j":
		next.Sel = (next.Sel + 1) % len(rows)
		next.ArmDelete = false
		return next, "move"
	case "ctrl+n":
		return next, "new"
	case "ctrl+r":
		return next, "refresh"
	case "ctrl+d":
		// Close live session (only valid on a live row).
		row := rows[next.Sel]
		if row.Kind == SessionRowLive {
			return next, "close-live"
		}
		return next, "noop"
	case "d":
		row := rows[next.Sel]
		if row.Kind == SessionRowHistory {
			if next.ArmDelete {
				next.ArmDelete = false
				return next, "delete"
			}
			next.ArmDelete = true
			return next, "arm-delete"
		}
		return next, "noop"
	case "enter":
		row := rows[next.Sel]
		switch row.Kind {
		case SessionRowNew:
			return next, "new"
		case SessionRowLive:
			return next, "switch-live"
		case SessionRowHistory:
			return next, "resume"
		}
	case "esc":
		if next.ArmDelete {
			next.ArmDelete = false
			return next, "cancel"
		}
		return next, "cancel"
	}
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		next.Filter += key
		return next, "filter"
	}
	if key == "backspace" && len(next.Filter) > 0 {
		next.Filter = next.Filter[:len(next.Filter)-1]
		return next, "filter"
	}
	return next, "noop"
}

// reanchorSel re-resolves the cursor by Entry.ID after a live-session
// poll changes the list. Returns the new SessionSwitcherState with
// s.Sel pointing at the row whose Entry.ID matches the previously-
// selected one (or 0 if it disappeared).
func reanchorSel(s SessionSwitcherState, prevID string) SessionSwitcherState {
	rows := s.mergedRows()
	if prevID == "" {
		return s
	}
	for i, r := range rows {
		if r.Entry.ID == prevID {
			s.Sel = i
			return s
		}
	}
	return s
}
