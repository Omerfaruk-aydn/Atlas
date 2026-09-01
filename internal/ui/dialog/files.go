package dialog

import (
	"fmt"
	"path/filepath"

	"github.com/maincodss/atlas-agent/internal/deps/bubbles/v2/help"
	"github.com/maincodss/atlas-agent/internal/deps/bubbles/v2/key"
	tea "github.com/maincodss/atlas-agent/internal/deps/bubbletea/v2"
	"github.com/maincodss/atlas-agent/internal/ui/common"
	"github.com/maincodss/atlas-agent/internal/ui/list"
	uv "github.com/maincodss/atlas-agent/internal/deps/cb/ultraviolet"
)

// FilesID is the identifier for the modified-files dialog.
const FilesID = "files"

// FileDiffEntry is one modified file, carrying enough to open its diff
// without the dialog package depending on model.SessionFile (which would
// be an import cycle — model already imports dialog).
type FileDiffEntry struct {
	Path                 string
	Before, After        string
	Additions, Deletions int
}

// fileEntry is one row in the Files dialog.
type fileEntry struct {
	*list.Versioned
	entry   FileDiffEntry
	t       *common.Common
	focused bool
}

var _ list.Item = &fileEntry{}

func (e *fileEntry) Finished() bool { return true }

// Filter returns the filterable value for this entry.
func (e *fileEntry) Filter() string { return e.entry.Path }

func (e *fileEntry) SetFocused(focused bool) {
	if e.focused == focused {
		return
	}
	e.focused = focused
	if e.Versioned != nil {
		e.Bump()
	}
}

func (e *fileEntry) Render(width int) string {
	t := e.t.Styles
	itemStyles := ListItemStyles{
		ItemBlurred:     t.Dialog.NormalItem,
		ItemFocused:     t.Dialog.SelectedItem,
		InfoTextBlurred: t.Dialog.Sessions.InfoBlurred,
		InfoTextFocused: t.Dialog.Sessions.InfoFocused,
	}
	var stats string
	if e.entry.Additions > 0 {
		stats += t.Files.Additions.Render(fmt.Sprintf("+%d", e.entry.Additions)) + " "
	}
	if e.entry.Deletions > 0 {
		stats += t.Files.Deletions.Render(fmt.Sprintf("-%d", e.entry.Deletions))
	}
	return renderItem(itemStyles, filepath.Base(e.entry.Path), stats, e.focused, width, nil, nil)
}

// Files lists the current session's modified files; selecting one opens its
// cumulative diff (session start to latest version).
type Files struct {
	com  *common.Common
	help help.Model
	list *list.FilterableList

	keyMap struct {
		Next     key.Binding
		Previous key.Binding
		Select   key.Binding
		Close    key.Binding
	}
}

var _ Dialog = (*Files)(nil)

// NewFiles creates a new Files dialog from the sidebar's already-memoized
// modified-file list.
func NewFiles(com *common.Common, entries []FileDiffEntry) *Files {
	d := &Files{com: com}

	items := make([]list.FilterableItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, &fileEntry{Versioned: list.NewVersioned(), entry: e, t: com})
	}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.list = list.NewFilterableList(items...)
	d.list.Focus()

	d.keyMap.Next = key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "next"))
	d.keyMap.Previous = key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "previous"))
	d.keyMap.Select = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "view diff"))
	d.keyMap.Close = CloseKey

	return d
}

// ID implements Dialog.
func (d *Files) ID() string {
	return FilesID
}

// HandleMsg implements Dialog.
func (d *Files) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, d.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, d.keyMap.Previous):
		if d.list.IsSelectedFirst() {
			d.list.SelectLast()
		} else {
			d.list.SelectPrev()
		}
		d.list.ScrollToSelected()
	case key.Matches(keyMsg, d.keyMap.Next):
		if d.list.IsSelectedLast() {
			d.list.SelectFirst()
		} else {
			d.list.SelectNext()
		}
		d.list.ScrollToSelected()
	case key.Matches(keyMsg, d.keyMap.Select):
		item := d.list.SelectedItem()
		if item == nil {
			return nil
		}
		fe, ok := item.(*fileEntry)
		if !ok {
			return nil
		}
		return ActionOpenFileDiff{Entry: fe.entry}
	}
	return nil
}

// Cursor implements Dialog. The files dialog has no text input.
func (d *Files) Cursor() *tea.Cursor {
	return nil
}

// Draw implements [Dialog].
func (d *Files) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(defaultDialogMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(defaultDialogHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	rc := NewRenderContext(t, width)
	rc.Title = "Modified Files"

	if len(d.list.FilteredItems()) == 0 {
		rc.AddPart(t.Dialog.Sessions.RenamingingMessage.Render("No modified files in this session."))
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
func (d *Files) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Previous, d.keyMap.Next, d.keyMap.Select, d.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (d *Files) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
