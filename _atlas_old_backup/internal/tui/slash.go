package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// SlashCommand is the metadata + handler for one "/" command. Run may
// return a tea.Cmd to schedule an async follow-up (e.g. model list
// fetch) and/or a string to inject back into the input (e.g. /clear
// resetting state). It returns a note to display and a tea.Cmd to run.
type SlashCommand struct {
	Name        string   // canonical name, no leading slash, e.g. "model"
	Aliases     []string // alternative names ("/m" → "model")
	Group       string   // display group label ("Çekirdek", "Oturum", …)
	Help        string   // one-line summary
	Usage       string   // optional usage hint shown in /help
	Description string   // longer prose, matched at tier+3 in fuzzy search
	Run         SlashRun // handler
}

// SlashRun executes a slash command. arg is whatever the user typed after
// the command name (e.g. for "/model gpt-4" arg is "gpt-4"). The handler
// returns a "note" string to display to the user (a one-line system message
// about what happened — or empty for silence) and a tea.Cmd for any
// async follow-up. It may also return a "prefilled" string to be set as
// the new input value (e.g. /paste from clipboard).
type SlashRun func(arg string, app *App) (note string, cmd tea.Cmd, prefilled string)

// teaCmd is a true alias (=, not a type definition) of tea.Cmd so the
// SlashRun signature can use it without forcing every handler file to
// import bubbletea.
type teaCmd = tea.Cmd

// SlashRegistry holds the master command list and the lookup map built
// lazily from it. Commands are organized by Group; the registry preserves
// registration order for the /help display and for stable fuzzy-sort
// tie-breaking.
type SlashRegistry struct {
	all     []SlashCommand
	byName  map[string]*SlashCommand
	byGroup map[string][]SlashCommand
}

func newSlashRegistry(groups ...[]SlashCommand) *SlashRegistry {
	r := &SlashRegistry{
		byName:  make(map[string]*SlashCommand),
		byGroup: make(map[string][]SlashCommand),
	}
	for _, g := range groups {
		for _, c := range g {
			r.all = append(r.all, c)
			r.byGroup[c.Group] = append(r.byGroup[c.Group], c)
			r.byName[strings.ToLower(c.Name)] = &r.all[len(r.all)-1]
			for _, a := range c.Aliases {
				r.byName[strings.ToLower(a)] = &r.all[len(r.all)-1]
			}
		}
	}
	return r
}

// Find looks up a command by name or alias. The match is case-insensitive.
// Returns nil if the name is unknown.
func (r *SlashRegistry) Find(name string) *SlashCommand {
	if name == "" {
		return nil
	}
	return r.byName[strings.ToLower(strings.TrimPrefix(name, "/"))]
}

// Sorted returns the command list in stable registration order.
func (r *SlashRegistry) Sorted() []SlashCommand {
	return r.all
}

// Grouped returns commands grouped by their Group field, with each group
// sorted by name. Useful for the /help popup's sectioned display.
func (r *SlashRegistry) Grouped() (groups []string, byGroup map[string][]SlashCommand) {
	byGroup = make(map[string][]SlashCommand, len(r.byGroup))
	for g, list := range r.byGroup {
		cp := make([]SlashCommand, len(list))
		copy(cp, list)
		sort.SliceStable(cp, func(i, j int) bool { return cp[i].Name < cp[j].Name })
		byGroup[g] = cp
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups, byGroup
}

// MatchFilter is a typed list of FuzzyScoreItems for the slash-command
// fuzzy matcher, built from the registry. Pass it to rankFuzzy.
func (r *SlashRegistry) MatchFilter() []FuzzyScoreItem {
	out := make([]FuzzyScoreItem, 0, len(r.all))
	for _, c := range r.all {
		out = append(out, FuzzyScoreItem{
			ID:          c.Name,
			Aliases:     c.Aliases,
			Label:       c.Name,
			Description: c.Help + " " + c.Description,
		})
	}
	return out
}

// parseSlashInput splits raw input like "/model gpt-4" into (name, arg).
// Leading slash is optional. Empty input returns ("", "").
func parseSlashInput(raw string) (name, arg string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "/")
	if raw == "" {
		return "", ""
	}
	if i := strings.IndexAny(raw, " \t"); i >= 0 {
		return raw[:i], strings.TrimSpace(raw[i+1:])
	}
	return raw, ""
}

// completionToApplyOnSubmit encodes Hermes's "Enter must not be eaten by a
// popover that would just append a trailing space" rule: if the only thing
// the popover would change is to add a trailing space to an otherwise
// complete command, return ("", -1) so the caller submits instead. Returns
// (name, -1) when the completion is a no-op.
func completionToApplyOnSubmit(currentInput, completion string) (apply string, isNoOp bool) {
	cur := strings.TrimSpace(currentInput)
	comp := strings.TrimSpace(completion)
	if comp == "" {
		return "", true
	}
	if cur == comp {
		return "", true
	}
	// Trailing-space-only addition: the popover would only append a space
	// to an already-complete command. Don't swallow Enter.
	if cur == comp && strings.HasSuffix(currentInput, " ") == false &&
		strings.HasSuffix(completion, " ") {
		return "", true
	}
	return completion, false
}

// formatSlashList renders a flat command list as "/name (alias1, alias2) — help".
// Used by /help and the suggestion dropdown description column.
func formatSlashList(cmd SlashCommand) string {
	head := "/" + cmd.Name
	if len(cmd.Aliases) > 0 {
		head += " ("
		for i, a := range cmd.Aliases {
			if i > 0 {
				head += ", "
			}
			head += "/" + a
		}
		head += ")"
	}
	if cmd.Help != "" {
		return fmt.Sprintf("%-18s — %s", head, cmd.Help)
	}
	return head
}
