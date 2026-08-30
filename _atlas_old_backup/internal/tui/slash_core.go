package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/omerfarukaydin/atlas/internal/llm"
)

// coreCommands is the "Çekirdek" (Core) command group: the few commands
// every session needs. Mirrors Hermes's core.ts group of the same name.
func coreCommands(a *appForRegistry) []SlashCommand {
	return []SlashCommand{
		{
			Name:        "help",
			Aliases:     []string{"h", "?"},
			Group:       "Çekirdek",
			Help:        "Komutları ve kısayolları göster",
			Usage:       "/help",
			Description: "Mevcut slash komutlarını ve klavye kısayollarını listeler.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				cmds, byGroup := app.slash.Grouped()
				var b strings.Builder
				b.WriteString("Komutlar (gruplanmış):\n")
				for _, g := range cmds {
					b.WriteString("  " + g + ":\n")
					for _, c := range byGroup[g] {
						b.WriteString("    " + formatSlashList(c) + "\n")
					}
				}
				b.WriteString("\nKısayollar:\n")
				for _, h := range helpHintPairs {
					b.WriteString("  " + h.Keys + "  — " + h.Desc + "\n")
				}
				return b.String(), nil, ""
			},
		},
		{
			Name:        "clear",
			Aliases:     []string{"c"},
			Group:       "Çekirdek",
			Help:        "Görüşme geçmişini temizle",
			Usage:       "/clear",
			Description: "Bellekteki tüm mesajları siler; yeni bir sohbet başlatır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				app.messages = nil
				app.render()
				return "Görüşme temizlendi.", nil, ""
			},
		},
		{
			Name:        "provider",
			Aliases:     []string{"p"},
			Group:       "Çekirdek",
			Help:        "LLM sağlayıcısını değiştir",
			Usage:       "/provider [ad]",
			Description: "Sağlayıcıyı değiştirir; argümansız ok tuşlarıyla seçim menüsü açar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if arg != "" {
					return "Sağlayıcı değiştirildi: " + arg, nil, ""
				}
				items := make([]pickerItem, 0, len(llm.Available()))
				for _, name := range llm.Available() {
					desc := "LLM sağlayıcısı"
					if name == a.ag.ProviderName() {
						desc = "şu an aktif"
					}
					items = append(items, pickerItem{name: name, desc: desc})
				}
				app.openPicker("Sağlayıcı seç", pickerKindProvider, items)
				return "", nil, ""
			},
		},
		{
			Name:        "model",
			Aliases:     []string{"m"},
			Group:       "Çekirdek",
			Help:        "Modeli değiştir",
			Usage:       "/model [ad]",
			Description: "Aktif modeli değiştirir; argümansız sağlayıcıdan model listesi çeker.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if arg != "" {
					app.agent.SetModel(arg)
					return "Model değiştirildi: " + arg, nil, ""
				}
				return "Modeller getiriliyor (" + app.agent.ProviderName() + ")...",
					func() tea.Msg { return listModelsCmd(app.agent)() },
					""
			},
		},
		{
			Name:        "sessions",
			Aliases:     []string{"s"},
			Group:       "Çekirdek",
			Help:        "Aktif oturumları listele / değiştir",
			Usage:       "/sessions",
			Description: "Mevcut oturumlar arasında geçiş yap; yenilerini başlat.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				app.openSessionSwitcher()
				return "", nil, ""
			},
		},
		{
			Name:        "welcome",
			Aliases:     []string{"w"},
			Group:       "Çekirdek",
			Help:        "Karşılama panelini tekrar göster",
			Usage:       "/welcome",
			Description: "Sağlayıcı, model, araçlar özetini içeren başlangıç panelini tekrar açar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				app.showWelcome = true
				app.render()
				return "Hoş geldin paneli açıldı.", nil, ""
			},
		},
		{
			Name:        "exit",
			Aliases:     []string{"quit", "q"},
			Group:       "Çekirdek",
			Help:        "Atlas'tan çık",
			Usage:       "/exit",
			Description: "Aktif oturumu kapatır; güle güle mesajı yazdırır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return app.theme.GoodbyeMessage, func() tea.Msg { return tea.Quit }, ""
			},
		},
	}
}
