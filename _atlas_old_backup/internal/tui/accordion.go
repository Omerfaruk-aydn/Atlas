package tui

import "fmt"

// accordion is Hermes Agent's "single expand/collapse primitive" — reused
// for every collapsible section (session info, tool lists, long output)
// instead of writing bespoke toggle logic per panel. Ported as a stateless
// render helper since Atlas's App already owns all UI state directly.
type accordion struct {
	title  string
	count  *int   // shown as "(N)" after the title when set
	suffix string // shown after the count, e.g. "in 3 categories"
	open   bool
	body   string // rendered only when open
}

func (a *App) renderAccordion(acc accordion) string {
	chevron := "▸"
	if acc.open {
		chevron = "▾"
	}

	header := chevron + " " + acc.title
	if acc.count != nil {
		header += fmt.Sprintf(" (%d)", *acc.count)
	}
	if acc.suffix != "" {
		header += " " + acc.suffix
	}
	header = a.theme.Title.Render(header)

	if !acc.open || acc.body == "" {
		return header
	}
	return header + "\n" + acc.body
}
