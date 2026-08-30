package tui

import "strings"

// StickyPromptLabel returns the single-line, whitespace-collapsed label
// for a user message that has scrolled above the visible viewport. The
// user is mid-way through a long assistant reply; showing "you're mid-reply
// to: <this>" is a low-cost orientation aid.
//
// Hermes's stickyPromptFromViewport binary-searches into a prefix-offset
// array to find the visible range. Atlas's chat uses bubbles/viewport,
// which doesn't expose a per-line offset array — we approximate by
// checking whether the latest user message's *bubble* row is still
// visible based on the messages list length and chat height. The
// approximation is good enough for the "do I show a sticky label at all"
// decision, which is the only thing that matters for the breadcrumb's
// presence.
func (a *App) stickyPromptLabel() string {
	if !a.streaming {
		// Sticky only while a turn is active — otherwise the transcript
		// is at rest and the breadcrumb would be visual noise.
		return ""
	}

	// Walk messages backward to find the latest user message.
	var lastUser string
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].role == "user" {
			lastUser = a.messages[i].text
			break
		}
	}
	if lastUser == "" {
		return ""
	}

	// Only show the breadcrumb if the chat has scrolled away from the
	// bottom. Atlas's viewport model doesn't expose "is at bottom"
	// cheaply; we approximate by looking at how many info/tool rows have
	// accumulated since the last user message — if any, the chat is
	// long enough that the user may have scrolled.
	scrollAway := false
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].role == "user" {
			break
		}
		scrollAway = true
	}
	if !scrollAway {
		return ""
	}

	return collapseForSticky(lastUser)
}

// collapseForSticky reduces a potentially multi-line user message to a
// single line, with internal whitespace collapsed to a single space and
// capped at stickyMaxLen characters so the breadcrumb never displaces
// useful chrome.
const stickyMaxLen = 80

func collapseForSticky(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const ellipsis = "…"
	if len(s) > stickyMaxLen {
		// Reserve room for the multi-byte ellipsis so the final
		// string lands AT stickyMaxLen bytes, not over.
		s = s[:stickyMaxLen-len(ellipsis)] + ellipsis
	}
	return s
}

// renderStickyPrompt draws the "↳ <label>" breadcrumb that appears above
// the composer when the chat has scrolled away from the active user
// message. Returns "" when there's nothing to show.
func (a *App) renderStickyPrompt() string {
	label := a.stickyPromptLabel()
	if label == "" {
		return ""
	}
	return a.theme.HelpText.Render("↳ " + label)
}
