package dialog

import (
	"fmt"
	"sort"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	tea "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ui/v2"
	uv "github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-ultraviolet"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-widgets/v2/help"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-widgets/v2/key"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/list"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/util"
)

// ModelRolesID is the identifier for the model roles dialog.
const ModelRolesID = "model_roles"

// modelRoleEntry is one row: a named model role and the provider/model
// it resolves to. The two built-in roles (large/small) are shown for
// reference but cannot be edited or deleted here -- they already have
// their own dedicated Models dialog.
type modelRoleEntry struct {
	*list.Versioned
	name    string
	model   config.SelectedModel
	builtin bool
	t       *common.Common
	focused bool
}

var _ list.Item = &modelRoleEntry{}

func (e *modelRoleEntry) Finished() bool { return true }

func (e *modelRoleEntry) Filter() string { return e.name }

func (e *modelRoleEntry) info() string {
	info := fmt.Sprintf("%s / %s", e.model.Provider, e.model.Model)
	if e.model.ReasoningEffort != "" {
		info += " · " + e.model.ReasoningEffort + " reasoning"
	}
	if e.builtin {
		info += " (built-in)"
	}
	return info
}

func (e *modelRoleEntry) SetFocused(focused bool) {
	if e.focused == focused {
		return
	}
	e.focused = focused
	if e.Versioned != nil {
		e.Bump()
	}
}

func (e *modelRoleEntry) Render(width int) string {
	t := e.t.Styles
	itemStyles := ListItemStyles{
		ItemBlurred:     t.Dialog.NormalItem,
		ItemFocused:     t.Dialog.SelectedItem,
		InfoTextBlurred: t.Dialog.Sessions.InfoBlurred,
		InfoTextFocused: t.Dialog.Sessions.InfoFocused,
	}
	return renderItem(itemStyles, e.name, e.info(), e.focused, width, nil, nil)
}

// ModelRoles lists the model roles a subagent's model field, the
// advisor, and vibe workers can reference by name (see
// internal/config's ModelRoles and `atlas models roles`): the two
// built-in ones (large/small) plus any custom roles configured. From
// here a custom role can be added, edited, or removed without hand
// editing the config file.
type ModelRoles struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	keyMap struct {
		Next     key.Binding
		Previous key.Binding
		Add      key.Binding
		Edit     key.Binding
		Delete   key.Binding
		Close    key.Binding
	}
}

var _ Dialog = (*ModelRoles)(nil)

// NewModelRoles creates a new ModelRoles dialog, reading the current
// role set synchronously from com.Config() -- already a live, locally
// cached snapshot in both local and client/server mode (see
// Workspace.Config), so no separate fetch is needed.
func NewModelRoles(com *common.Common) *ModelRoles {
	d := &ModelRoles{com: com}
	d.list = list.NewFilterableList(d.buildItems()...)
	d.list.Focus()
	if d.list.Len() > 0 {
		d.list.SelectFirst()
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.keyMap.Next = key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "next"))
	d.keyMap.Previous = key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "previous"))
	d.keyMap.Add = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add"))
	d.keyMap.Edit = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit"))
	d.keyMap.Delete = key.NewBinding(key.WithKeys("x", "ctrl+x"), key.WithHelp("x", "delete"))
	d.keyMap.Close = CloseKey

	return d
}

// buildItems reads the current role set: large/small (builtin) first,
// then custom roles sorted by name.
func (d *ModelRoles) buildItems() []list.FilterableItem {
	cfg := d.com.Config()
	if cfg == nil {
		return nil
	}

	var items []list.FilterableItem
	for _, t := range []config.SelectedModelType{config.SelectedModelTypeLarge, config.SelectedModelTypeSmall} {
		if model, ok := cfg.Models[t]; ok {
			items = append(items, &modelRoleEntry{Versioned: list.NewVersioned(), name: string(t), model: model, builtin: true, t: d.com})
		}
	}

	var names []string
	var roles map[string]config.SelectedModel
	if cfg.Options != nil {
		roles = cfg.Options.ModelRoles
	}
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		items = append(items, &modelRoleEntry{Versioned: list.NewVersioned(), name: name, model: roles[name], t: d.com})
	}
	return items
}

// Refresh rebuilds the list from the current config -- called after a
// save or delete completes, so the dialog reflects its own change
// without the user having to close and reopen it.
func (d *ModelRoles) Refresh() {
	selected := ""
	if item, ok := d.list.SelectedItem().(*modelRoleEntry); ok {
		selected = item.name
	}
	d.list.SetItems(d.buildItems()...)
	for i, item := range d.list.FilteredItems() {
		if e, ok := item.(*modelRoleEntry); ok && e.name == selected {
			d.list.SetSelected(i)
			break
		}
	}
}

// ID implements Dialog.
func (d *ModelRoles) ID() string {
	return ModelRolesID
}

// HandleMsg implements Dialog.
func (d *ModelRoles) HandleMsg(msg tea.Msg) Action {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, d.keyMap.Close):
			return ActionClose{}
		case key.Matches(msg, d.keyMap.Previous):
			if d.list.IsSelectedFirst() {
				d.list.SelectLast()
			} else {
				d.list.SelectPrev()
			}
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Next):
			if d.list.IsSelectedLast() {
				d.list.SelectFirst()
			} else {
				d.list.SelectNext()
			}
			d.list.ScrollToSelected()
		case key.Matches(msg, d.keyMap.Add):
			return ActionOpenModelRoleForm{}
		case key.Matches(msg, d.keyMap.Edit):
			entry, ok := d.selectedCustomEntry()
			if !ok {
				return nil
			}
			return ActionOpenModelRoleForm{
				ExistingName:            entry.name,
				ExistingProvider:        entry.model.Provider,
				ExistingModel:           entry.model.Model,
				ExistingReasoningEffort: entry.model.ReasoningEffort,
			}
		case key.Matches(msg, d.keyMap.Delete):
			entry, ok := d.selectedCustomEntry()
			if !ok {
				return nil
			}
			return ActionCmd{d.deleteCmd(entry.name)}
		}
	case modelRoleDeletedMsg:
		if msg.err != nil {
			return ActionCmd{util.ReportError(fmt.Errorf("failed to delete role %q: %w", msg.name, msg.err))}
		}
		d.Refresh()
	}
	return nil
}

// modelRoleDeletedMsg delivers the async result of deleting a custom
// model role.
type modelRoleDeletedMsg struct {
	name string
	err  error
}

func (d *ModelRoles) deleteCmd(name string) tea.Cmd {
	ws := d.com.Workspace
	return func() tea.Msg {
		err := ws.RemoveConfigField(config.ScopeGlobal, "options.model_roles."+name)
		return modelRoleDeletedMsg{name: name, err: err}
	}
}

// selectedCustomEntry returns the selected row if it is a deletable
// custom role, not one of the built-in large/small entries.
func (d *ModelRoles) selectedCustomEntry() (*modelRoleEntry, bool) {
	item := d.list.SelectedItem()
	if item == nil {
		return nil, false
	}
	entry, ok := item.(*modelRoleEntry)
	if !ok || entry.builtin {
		return nil, false
	}
	return entry, true
}

// Cursor implements Dialog. The model roles list has no text input of
// its own; add/edit happen in a separate Arguments form.
func (d *ModelRoles) Cursor() *tea.Cursor {
	return nil
}

// Draw implements [Dialog].
func (d *ModelRoles) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	rc := NewRenderContext(t, width)
	rc.Title = "Model Roles"

	if d.list.Len() == 0 {
		rc.AddPart(t.Dialog.Sessions.RenamingingMessage.Render("No model roles configured yet."))
	} else {
		listHeight, listTotalHeight, _ := sizeDialogList(t, d.list, innerWidth, height)
		bodyView := t.Dialog.List.Height(d.list.Height()).Render(d.list.Render())
		bodyView = joinScrollbar(t, bodyView, listHeight, listTotalHeight, listHeight, d.list.Offset())
		rc.AddPart(bodyView)
	}

	rc.Help = renderDialogHelp(t, &d.help, d, innerWidth)

	view := rc.Render()
	DrawCenterCursor(scr, area, view, nil)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *ModelRoles) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Previous, d.keyMap.Next, d.keyMap.Add, d.keyMap.Edit, d.keyMap.Delete, d.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (d *ModelRoles) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
