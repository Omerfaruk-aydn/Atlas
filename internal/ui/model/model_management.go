package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/commands"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	editor "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-editor"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/dialog"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/util"
)

// This file wires the Model Roles, Model Fallbacks, and Subagents
// management dialogs into the UI: opening each list, transitioning
// into and back out of the shared Arguments form for add/edit, and
// (for subagents) handing the instructions body off to $EDITOR. Each
// list dialog handles its own delete flow internally (see
// dialog.ModelRoles/Fallbacks/Subagents); only the cross-dialog
// transitions live here, mirroring how ActionOpenFileDiff hands off
// from Files to FileDiff elsewhere in ui.go.

func (m *UI) openModelRolesDialog() tea.Cmd {
	if m.dialog.ContainsDialog(dialog.ModelRolesID) {
		m.dialog.BringToFront(dialog.ModelRolesID)
		return nil
	}
	m.dialog.OpenDialog(dialog.NewModelRoles(m.com))
	return nil
}

func (m *UI) openFallbacksDialog() tea.Cmd {
	if m.dialog.ContainsDialog(dialog.FallbacksID) {
		m.dialog.BringToFront(dialog.FallbacksID)
		return nil
	}
	m.dialog.OpenDialog(dialog.NewFallbacks(m.com))
	return nil
}

func (m *UI) openSubagentsDialog() tea.Cmd {
	if m.dialog.ContainsDialog(dialog.SubagentsID) {
		m.dialog.BringToFront(dialog.SubagentsID)
		return nil
	}
	m.dialog.OpenDialog(dialog.NewSubagents(m.com))
	return nil
}

// -- Model roles --

func (m *UI) handleOpenModelRoleForm(msg dialog.ActionOpenModelRoleForm) tea.Cmd {
	m.dialog.CloseDialog(dialog.ModelRolesID)

	var args []commands.Argument
	title := "New Model Role"
	if msg.ExistingName == "" {
		args = append(args, commands.Argument{ID: "name", Title: "Name", Description: "e.g. research", Required: true})
	} else {
		title = "Edit Role: " + msg.ExistingName
	}
	args = append(args,
		commands.Argument{ID: "provider", Title: "Provider", Description: "e.g. openai", Required: true},
		commands.Argument{ID: "model", Title: "Model", Description: "e.g. gpt-4o", Required: true},
	)

	form := dialog.NewArguments(m.com, title, "A named model role a subagent, the advisor, or a vibe worker can run on -- see `atlas models roles`.",
		args, dialog.ActionSaveModelRole{ExistingName: msg.ExistingName})
	if msg.ExistingName != "" {
		form.SetValues(map[string]string{"provider": msg.ExistingProvider, "model": msg.ExistingModel})
	}
	m.dialog.OpenDialog(form)
	return nil
}

// modelRoleSavedMsg delivers the async result of writing a model role.
type modelRoleSavedMsg struct {
	err error
}

func (m *UI) handleSaveModelRole(msg dialog.ActionSaveModelRole) tea.Cmd {
	m.dialog.CloseDialog(dialog.ArgumentsID)

	name := strings.TrimSpace(msg.ExistingName)
	if name == "" {
		name = strings.TrimSpace(msg.Args["name"])
	}
	if name == "" {
		return util.ReportWarn("Role name is required.")
	}
	model := config.SelectedModel{
		Provider: strings.TrimSpace(msg.Args["provider"]),
		Model:    strings.TrimSpace(msg.Args["model"]),
	}

	ws := m.com.Workspace
	return func() tea.Msg {
		err := ws.SetConfigField(config.ScopeGlobal, "options.model_roles."+name, model)
		return modelRoleSavedMsg{err: err}
	}
}

// -- Model fallbacks --

func (m *UI) handleOpenFallbackEntryForm(msg dialog.ActionOpenFallbackEntryForm) tea.Cmd {
	m.dialog.CloseDialog(dialog.FallbacksID)
	form := dialog.NewArguments(m.com, "Add Fallback: "+string(msg.ModelType),
		"Appended to the end of "+string(msg.ModelType)+"'s fallback chain -- tried in order when the primary model hits a 429.",
		[]commands.Argument{
			{ID: "provider", Title: "Provider", Description: "e.g. openai", Required: true},
			{ID: "model", Title: "Model", Description: "e.g. gpt-4o-mini", Required: true},
		},
		dialog.ActionSaveFallbackEntry{ModelType: msg.ModelType},
	)
	m.dialog.OpenDialog(form)
	return nil
}

// fallbackEntrySavedMsg delivers the async result of appending a
// fallback entry.
type fallbackEntrySavedMsg struct {
	err error
}

func (m *UI) handleSaveFallbackEntry(msg dialog.ActionSaveFallbackEntry) tea.Cmd {
	m.dialog.CloseDialog(dialog.ArgumentsID)

	entry := config.SelectedModel{
		Provider: strings.TrimSpace(msg.Args["provider"]),
		Model:    strings.TrimSpace(msg.Args["model"]),
	}

	var current []config.SelectedModel
	if cfg := m.com.Config(); cfg != nil && cfg.Options != nil {
		current = cfg.Options.ModelFallbacks[msg.ModelType]
	}
	updated := append(append([]config.SelectedModel{}, current...), entry)

	ws := m.com.Workspace
	return func() tea.Msg {
		err := ws.SetConfigField(config.ScopeGlobal, "options.model_fallbacks."+string(msg.ModelType), updated)
		return fallbackEntrySavedMsg{err: err}
	}
}

func (m *UI) handleOpenFallbackCooldownForm(msg dialog.ActionOpenFallbackCooldownForm) tea.Cmd {
	m.dialog.CloseDialog(dialog.FallbacksID)
	form := dialog.NewArguments(m.com, "Fallback Cooldown",
		"Seconds a fallback stays active after a failover before the next turn returns to the primary model. 0 returns every turn.",
		[]commands.Argument{
			{ID: "seconds", Title: "Seconds", Description: "e.g. 300", Required: true},
		},
		dialog.ActionSaveFallbackCooldown{},
	)
	form.SetValues(map[string]string{"seconds": strconv.Itoa(msg.Current)})
	m.dialog.OpenDialog(form)
	return nil
}

func (m *UI) handleSaveFallbackCooldown(msg dialog.ActionSaveFallbackCooldown) tea.Cmd {
	m.dialog.CloseDialog(dialog.ArgumentsID)

	seconds, err := strconv.Atoi(strings.TrimSpace(msg.Args["seconds"]))
	if err != nil || seconds < 0 {
		return util.ReportWarn("Cooldown must be a non-negative number of seconds.")
	}

	ws := m.com.Workspace
	return func() tea.Msg {
		saveErr := ws.SetConfigField(config.ScopeGlobal, "options.fallback_cooldown", seconds)
		return fallbackEntrySavedMsg{err: saveErr}
	}
}

// -- Subagents --

func (m *UI) handleOpenSubagentForm(msg dialog.ActionOpenSubagentForm) tea.Cmd {
	m.dialog.CloseDialog(dialog.SubagentsID)

	var args []commands.Argument
	title := "New Subagent"
	if msg.ExistingName == "" {
		args = append(args, commands.Argument{ID: "name", Title: "Name", Description: "e.g. research", Required: true})
	} else {
		title = "Edit Subagent: " + msg.ExistingName
	}
	args = append(args,
		commands.Argument{ID: "description", Title: "Description", Description: "when this subagent should be used", Required: true},
		commands.Argument{ID: "model", Title: "Model Role", Description: "e.g. research; empty runs on the session's model"},
	)

	description := "Instructions are edited separately, in $EDITOR, after saving (see enter on the subagents list)."
	form := dialog.NewArguments(m.com, title, description, args,
		dialog.ActionSaveSubagentMeta{ExistingName: msg.ExistingName, UserScope: msg.UserScope})
	if msg.ExistingName != "" {
		form.SetValues(map[string]string{"description": msg.ExistingDescription, "model": msg.ExistingModel})
	}
	m.dialog.OpenDialog(form)
	return nil
}

// subagentSavedMsg delivers the async result of saving a subagent's
// metadata.
type subagentSavedMsg struct {
	err error
}

// newSubagentInstructionsTemplate mirrors atlas agent new's own
// placeholder body (internal/cmd/agent_new.go), so a subagent created
// from the UI reads the same way as one scaffolded from the CLI until
// its instructions are actually written.
func newSubagentInstructionsTemplate(name string) string {
	return fmt.Sprintf("# %s\n\nTODO: the instructions this subagent runs with.\n", name)
}

func (m *UI) handleSaveSubagentMeta(msg dialog.ActionSaveSubagentMeta) tea.Cmd {
	m.dialog.CloseDialog(dialog.ArgumentsID)

	name := strings.TrimSpace(msg.ExistingName)
	isNew := name == ""
	if isNew {
		name = strings.TrimSpace(msg.Args["name"])
	}
	if name == "" {
		return util.ReportWarn("Subagent name is required.")
	}
	description := strings.TrimSpace(msg.Args["description"])
	if description == "" {
		return util.ReportWarn("Subagent description is required.")
	}

	sub := subagents.Subagent{
		Name:        name,
		Description: description,
		Model:       strings.TrimSpace(msg.Args["model"]),
	}
	if isNew {
		sub.Instructions = newSubagentInstructionsTemplate(name)
	}

	ws := m.com.Workspace
	return func() tea.Msg {
		if isNew {
			_, err := ws.SaveSubagent(context.Background(), sub, msg.UserScope)
			return subagentSavedMsg{err: err}
		}
		// Editing: preserve the existing instructions body instead of
		// overwriting it with the placeholder template, which is only
		// used for a genuinely new subagent.
		existing, err := ws.ListSubagents(context.Background())
		if err != nil {
			return subagentSavedMsg{err: err}
		}
		for _, e := range existing {
			if e.Name == name {
				sub.Instructions = e.Instructions
				break
			}
		}
		_, err = ws.SaveSubagent(context.Background(), sub, msg.UserScope)
		return subagentSavedMsg{err: err}
	}
}

// subagentFileEditedMsg signals that $EDITOR closed for a subagent's
// definition file, so the subagents list should refresh. Deliberately
// separate from openEditorMsg (used for the composer's own external-
// editor flow): that message always feeds its text into m.textarea,
// which is not what closing an unrelated file's editor should do.
type subagentFileEditedMsg struct{}

func (m *UI) handleEditSubagentFile(msg dialog.ActionEditSubagentFile) tea.Cmd {
	cmd, err := editor.Command("atlas", msg.Path)
	if err != nil {
		return util.ReportError(err)
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return util.ReportError(err)()
		}
		return subagentFileEditedMsg{}
	})
}

// refreshSubagentsDialogIfOpen re-lists subagents into the Subagents
// dialog if it is still open -- used after an $EDITOR round trip on a
// subagent's file, left open (unlike the save/delete flows, which
// close and reopen fresh) so the user lands back exactly where they
// were.
func (m *UI) refreshSubagentsDialogIfOpen() {
	if d, ok := m.dialog.Dialog(dialog.SubagentsID).(*dialog.Subagents); ok {
		d.Refresh()
	}
}
