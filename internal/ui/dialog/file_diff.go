package dialog

import (
	"fmt"
	"path/filepath"

	"github.com/maincodss/atlas-agent/internal/deps/bubbles/v2/help"
	"github.com/maincodss/atlas-agent/internal/deps/bubbles/v2/key"
	tea "github.com/maincodss/atlas-agent/internal/deps/bubbletea/v2"
	"github.com/maincodss/atlas-agent/internal/deps/lipgloss/v2"
	"github.com/maincodss/atlas-agent/internal/ui/common"
	uv "github.com/maincodss/atlas-agent/internal/deps/cb/ultraviolet"
)

// FileDiffID is the identifier for the sidebar file-diff dialog.
const FileDiffID = "file_diff"

// fileDiffMaxWidth/Height bound the dialog; it otherwise fills most of the
// screen since diffs need real space to be readable.
const (
	fileDiffMaxWidth  = 120
	fileDiffMaxHeight = 40
)

// FileDiff shows the cumulative diff (session start to latest version) for
// one file selected from the sidebar's "Modified Files" list.
type FileDiff struct {
	com           *common.Common
	path          string
	before, after string
	additions     int
	deletions     int
	yOffset       int

	keyMap struct {
		Up       key.Binding
		Down     key.Binding
		PageUp   key.Binding
		PageDown key.Binding
		Close    key.Binding
	}
	help help.Model
}

var _ Dialog = (*FileDiff)(nil)

// NewFileDiff creates a new FileDiff dialog. before/after are the file's
// content at the start and end of the session; additions/deletions are the
// precomputed line counts shown in the title.
func NewFileDiff(com *common.Common, path, before, after string, additions, deletions int) *FileDiff {
	d := &FileDiff{com: com, path: path, before: before, after: after, additions: additions, deletions: deletions}

	h := help.New()
	h.Styles = com.Styles.DialogHelpStyles()
	d.help = h

	d.keyMap.Up = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "scroll up"))
	d.keyMap.Down = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "scroll down"))
	d.keyMap.PageUp = key.NewBinding(key.WithKeys("pgup", "b"), key.WithHelp("pgup", "page up"))
	d.keyMap.PageDown = key.NewBinding(key.WithKeys("pgdown", "f", " "), key.WithHelp("pgdn", "page down"))
	d.keyMap.Close = CloseKey

	return d
}

// ID implements Dialog.
func (d *FileDiff) ID() string {
	return FileDiffID
}

// HandleMsg implements Dialog.
func (d *FileDiff) HandleMsg(msg tea.Msg) Action {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch {
	case key.Matches(keyMsg, d.keyMap.Close):
		return ActionClose{}
	case key.Matches(keyMsg, d.keyMap.Up):
		d.scroll(-1)
	case key.Matches(keyMsg, d.keyMap.Down):
		d.scroll(1)
	case key.Matches(keyMsg, d.keyMap.PageUp):
		d.scroll(-10)
	case key.Matches(keyMsg, d.keyMap.PageDown):
		d.scroll(10)
	}
	return nil
}

func (d *FileDiff) scroll(delta int) {
	d.yOffset = max(0, d.yOffset+delta)
}

// Cursor implements Dialog. The file diff view has no text input.
func (d *FileDiff) Cursor() *tea.Cursor {
	return nil
}

// Draw implements [Dialog].
func (d *FileDiff) Draw(scr uv.Screen, area uv.Rectangle) *tea.Cursor {
	t := d.com.Styles
	width := max(0, min(fileDiffMaxWidth, area.Dx()-t.Dialog.View.GetHorizontalBorderSize()))
	height := max(0, min(fileDiffMaxHeight, area.Dy()-t.Dialog.View.GetVerticalBorderSize()))
	innerWidth := width - t.Dialog.View.GetHorizontalFrameSize()

	helpView := renderDialogHelp(t, &d.help, d, innerWidth)
	bodyHeight := max(1, height-t.Dialog.View.GetVerticalFrameSize()-lipgloss.Height(helpView)-2)

	body := common.DiffFormatter(t).
		Unified().
		Before(filepath.Base(d.path), d.before).
		After(filepath.Base(d.path), d.after).
		Width(innerWidth).
		Height(bodyHeight).
		YOffset(d.yOffset).
		LineNumbers(true).
		String()

	rc := NewRenderContext(t, width)
	rc.Title = filepath.Base(d.path)
	rc.TitleInfo = t.Files.Additions.Render(fmt.Sprintf("+%d", d.additions)) +
		" " + t.Files.Deletions.Render(fmt.Sprintf("-%d", d.deletions))
	rc.Help = helpView
	rc.AddPart(body)

	view := rc.Render()
	DrawCenter(scr, area, view)
	return nil
}

// ShortHelp implements [help.KeyMap].
func (d *FileDiff) ShortHelp() []key.Binding {
	return []key.Binding{d.keyMap.Up, d.keyMap.Down, d.keyMap.PageUp, d.keyMap.PageDown, d.keyMap.Close}
}

// FullHelp implements [help.KeyMap].
func (d *FileDiff) FullHelp() [][]key.Binding {
	return [][]key.Binding{d.ShortHelp()}
}
