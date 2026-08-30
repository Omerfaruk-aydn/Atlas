package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ModelPickerState implements Hermes's two-stage model picker:
// stage 1 is the provider list, stage 2 is the model list for the
// selected provider. If the active provider requires an API key, an
// inline "enter API key" stage is shown between (with a masked-bullet
// echo). A separate disconnect confirm sub-stage handles Ctrl+D on
// an authenticated provider.
type ModelPickerStage int

const (
	ModelPickerProvider ModelPickerStage = iota
	ModelPickerModel
	ModelPickerAPIKey
	ModelPickerDisconnect
	ModelPickerScope // "session" vs "global" toggle
)

// ModelPickerPersistScope is the persistence scope for the picker.
// Mirrors Hermes's TUI_SESSION_MODEL_FLAG behavior: when the user
// picks a model from the TUI, we force-scope to --session so a
// future /model --global isn't silently clobbered.
type ModelPickerPersistScope int

const (
	PersistScopeSession ModelPickerPersistScope = iota
	PersistScopeGlobal
)

// ModelPickerState is the full picker state. Filter is the fuzzy
// search term; Sel is the highlighted option; Stage tracks which
// sub-screen is active.
type ModelPickerState struct {
	Stage         ModelPickerStage
	Providers     []ProviderEntry
	Models        []string
	Provider      string // selected provider (after stage 1 → stage 2)
	Filter        string
	Sel           int
	APIKey        string // in-flight masked entry
	APIDisplay    string // the visible masked echo
	Scope         ModelPickerPersistScope
	Authenticated bool
}

// ProviderEntry is one row in the provider stage. The AuthType field
// matches Hermes's provider auth_type detection: "api_key" providers
// show the inline key-entry stage; "oauth" providers jump straight to
// the model list.
type ProviderEntry struct {
	Name        string
	DisplayName string
	AuthType    string // "api_key" | "oauth" | "none"
	Authenticated bool
	Current     bool // true for the currently-active provider
}

// renderModelPicker paints the current stage. The provider/model lists
// are windowed via windowOffset so the active selection is always
// visible.
func (a *App) renderModelPicker(p ModelPickerState) string {
	width := a.width - 4
	if width < 40 {
		width = 40
	}
	height := a.height - 6
	if height < 8 {
		height = 8
	}
	var title, body, hint string
	switch p.Stage {
	case ModelPickerProvider:
		title = "Sağlayıcı seç"
		body = a.renderProviderList(p, width-4, height)
		hint = providerStageHint()
	case ModelPickerModel:
		title = fmt.Sprintf("Model seç (%s)", p.Provider)
		body = a.renderModelList(p, width-4, height)
		hint = modelStageHint()
	case ModelPickerAPIKey:
		title = fmt.Sprintf("API anahtarı: %s", p.Provider)
		body = a.renderAPIKeyEntry(p, width-4)
		hint = apiKeyStageHint()
	case ModelPickerDisconnect:
		title = fmt.Sprintf("%s bağlantısını kes?", p.Provider)
		body = a.theme.HelpText.Render("Bu oturum için API anahtarı ~/.atlas/.env'den kaldırılacak.\nDevam edilsin mi?")
		hint = "y onayla · n/esc iptal"
	case ModelPickerScope:
		title = "Kapsam seç"
		body = a.theme.HelpText.Render("Bu değişiklik nerede saklansın?")
		hint = "ctrl+s oturum · ctrl+g global"
	}
	return a.renderDialog(dialogBox{
		title: title,
		body:  body,
		hint:  hint,
		width: width,
	})
}

func providerStageHint() string {
	return "↑/↓ seç · Tab filtre · Enter onayla · Ctrl+N yeni · Esc kapat"
}

func modelStageHint() string {
	return "↑/↓ seç · Tab filtre · Enter uygula · Ctrl+S oturum · Ctrl+G global · Esc geri"
}

func apiKeyStageHint() string {
	return "Anahtarı yaz (maskeli) · Enter kaydet · Esc iptal"
}

// renderProviderList lists providers, marking the current one and
// filtering by p.Filter.
func (a *App) renderProviderList(p ModelPickerState, width, viewportSize int) string {
	filtered := filterProviders(p.Providers, p.Filter)
	if p.Sel >= len(filtered) {
		p.Sel = 0
	}
	off := windowOffset(p.Sel, viewportSize, len(filtered))
	end := off + viewportSize
	if end > len(filtered) {
		end = len(filtered)
	}
	visible := filtered[off:end]
	if len(visible) == 0 {
		return a.theme.HelpText.Render("(eşleşen sağlayıcı yok)")
	}
	// Render as a useMenu list.
	rows := make([]menuRow, len(visible))
	for i, prov := range visible {
		glyph := "○"
		if prov.Authenticated {
			glyph = "●"
		}
		if prov.Current {
			glyph = "*"
		}
		desc := prov.AuthType
		if prov.Current {
			desc = "şu an aktif"
		}
		rows[i] = menuRow{Label: glyph + " " + prov.DisplayName, Desc: desc}
	}
	return a.renderMenuList(rows, p.Sel-off, width)
}

// renderModelList lists models, filtering by p.Filter.
func (a *App) renderModelList(p ModelPickerState, width, viewportSize int) string {
	filtered := fuzzyFilterStrings(p.Models, p.Filter)
	if p.Sel >= len(filtered) {
		p.Sel = 0
	}
	off := windowOffset(p.Sel, viewportSize, len(filtered))
	end := off + viewportSize
	if end > len(filtered) {
		end = len(filtered)
	}
	visible := filtered[off:end]
	if len(visible) == 0 {
		return a.theme.HelpText.Render("(eşleşen model yok)")
	}
	rows := make([]menuRow, len(visible))
	for i, m := range visible {
		rows[i] = menuRow{Label: m, Desc: ""}
	}
	return a.renderMenuList(rows, p.Sel-off, width)
}

// renderAPIKeyEntry paints the masked-bullet input. The display
// accumulates "•" for each character the user has typed.
func (a *App) renderAPIKeyEntry(p ModelPickerState, width int) string {
	var b strings.Builder
	b.WriteString(a.theme.HelpText.Render("API anahtarını yapıştır ya da yaz:"))
	b.WriteString("\n\n")
	masked := p.APIDisplay
	if masked == "" {
		masked = strings.Repeat("•", 0)
	}
	if len(masked) > width-8 {
		masked = masked[:width-8]
	}
	b.WriteString(lipgloss.NewStyle().Width(width-4).Render(masked + "▏"))
	b.WriteString("\n\n")
	b.WriteString(a.theme.HelpText.Render("Anahtar ~/.atlas/.env'e yazılacak. " +
		"Atlas yeniden başlatıldığında otomatik yüklenir."))
	return b.String()
}

// modelPickerKeyAction dispatches one key event for the picker. The
// returned (nextState, action) pair keeps the dispatch pure; the
// App.Update case calls this and routes the action.
func modelPickerKeyAction(p ModelPickerState, key string, viewportSize int) (ModelPickerState, string) {
	next := p
	switch p.Stage {
	case ModelPickerProvider:
		return dispatchFilterableList(next, key, len(p.Providers), viewportSize, "provider")
	case ModelPickerModel:
		return dispatchFilterableList(next, key, len(p.Models), viewportSize, "model")
	case ModelPickerAPIKey:
		return apiKeyKeyAction(next, key)
	case ModelPickerDisconnect:
		return disconnectKeyAction(next, key)
	case ModelPickerScope:
		return scopeKeyAction(next, key)
	}
	return next, "noop"
}

func dispatchFilterableList(p ModelPickerState, key string, total, viewportSize int, stage string) (ModelPickerState, string) {
	next := p
	switch key {
	case "up", "k":
		next.Sel--
		if next.Sel < 0 {
			next.Sel = total - 1
		}
		return next, "move"
	case "down", "j":
		next.Sel++
		if next.Sel >= total {
			next.Sel = 0
		}
		return next, "move"
	case "tab", "ctrl+u":
		// Clear filter (Ctrl+U mirrors Hermes's filter-clear).
		next.Filter = ""
		return next, "clear-filter"
	case "esc":
		return next, "back"
	case "enter":
		return next, "choose"
	case "backspace":
		if len(next.Filter) > 0 {
			next.Filter = next.Filter[:len(next.Filter)-1]
		}
		return next, "edit-filter"
	case "ctrl+n":
		return next, "new"
	}
	// Printable characters extend the filter.
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		next.Filter += key
		return next, "edit-filter"
	}
	return next, "noop"
}

func apiKeyKeyAction(p ModelPickerState, key string) (ModelPickerState, string) {
	next := p
	switch key {
	case "enter":
		return next, "save"
	case "esc", "ctrl+c":
		return next, "cancel"
	case "backspace":
		if len(next.APIKey) > 0 {
			next.APIKey = next.APIKey[:len(next.APIKey)-1]
			next.APIDisplay = strings.TrimRight(next.APIDisplay, "•")
		}
		return next, "edit"
	}
	if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
		next.APIKey += key
		if len(next.APIDisplay) < 40 {
			next.APIDisplay += "•"
		}
		return next, "edit"
	}
	return next, "noop"
}

func disconnectKeyAction(p ModelPickerState, key string) (ModelPickerState, string) {
	switch key {
	case "y", "Y", "enter":
		return p, "disconnect"
	}
	return p, "cancel"
}

func scopeKeyAction(p ModelPickerState, key string) (ModelPickerState, string) {
	next := p
	switch key {
	case "ctrl+s", "s":
		next.Scope = PersistScopeSession
		return next, "choose"
	case "ctrl+g", "g":
		next.Scope = PersistScopeGlobal
		return next, "choose"
	}
	return next, "noop"
}

// filterProviders returns the providers matching the given filter
// (substring, case-insensitive). Empty filter returns all.
func filterProviders(ps []ProviderEntry, filter string) []ProviderEntry {
	if filter == "" {
		return ps
	}
	var out []ProviderEntry
	low := strings.ToLower(filter)
	for _, p := range ps {
		if strings.Contains(strings.ToLower(p.DisplayName), low) ||
			strings.Contains(strings.ToLower(p.Name), low) {
			out = append(out, p)
		}
	}
	return out
}

// fuzzyFilterStrings applies the same tier-based fuzzy match to a
// string list and returns the matches in score order.
func fuzzyFilterStrings(items []string, filter string) []string {
	if filter == "" {
		return items
	}
	type scored struct {
		i int
		s int
	}
	var matches []scored
	for i, s := range items {
		if score := scoreFuzzyItem(FuzzyScoreItem{ID: s}, filter); score < 4 {
			matches = append(matches, scored{i, score})
		}
	}
	// Stable sort by (score, i).
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && (matches[j].s < matches[j-1].s ||
			(matches[j].s == matches[j-1].s && matches[j].i < matches[j-1].i)); j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = items[m.i]
	}
	return out
}
