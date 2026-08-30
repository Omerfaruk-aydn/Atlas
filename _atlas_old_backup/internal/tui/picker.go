package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

// pickerKind distinguishes what a selection in the active picker applies to.
type pickerKind string

const (
	pickerKindProvider pickerKind = "provider"
	pickerKindModel    pickerKind = "model"
)

// pickerItem is one selectable row in a provider/model picker list.
type pickerItem struct {
	name, desc string
}

func (i pickerItem) Title() string       { return i.name }
func (i pickerItem) Description() string { return i.desc }
func (i pickerItem) FilterValue() string { return i.name }

// newPicker builds a keyboard-navigable (↑/↓, Enter to select, Esc to
// cancel, "/" to filter) selection list for providers or models.
func (a *App) newPicker(title string, items []pickerItem, width, height int) list.Model {
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}

	delegate := list.NewDefaultDelegate()
	// Chip treatment for the active row: a background fill rather than
	// ANSI reverse-video (which renders inconsistently — e.g. as an opaque
	// block — on terminals with transparent or non-default backgrounds),
	// with its own foreground so it stays legible regardless of the
	// theme's default text color.
	delegate.Styles.SelectedTitle = lipgloss.NewStyle().
		Background(a.theme.SelectedBg).
		Foreground(a.theme.SelectedFg).
		Bold(true).
		Padding(0, 0, 0, 1)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedTitle.
		Bold(false).
		Foreground(a.theme.Muted)

	l := list.New(listItems, delegate, width, height)
	l.Title = title
	l.Styles.Title = a.theme.Title
	l.SetShowStatusBar(false)
	l.SetShowHelp(true)
	l.SetFilteringEnabled(true)
	return l
}
