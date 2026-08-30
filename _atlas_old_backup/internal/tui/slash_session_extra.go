package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// sessionExtraCommands is the second batch of session/control slash
// commands. The Hermes equivalent covers user-facing config toggles
// that aren't quite "core" but still need an entry point: theme,
// reasoning, busy policy, compress, voice, pet, etc.
func sessionExtraCommands(a *appForRegistry) []SlashCommand {
	return []SlashCommand{
		{
			Name:        "theme",
			Group:       "Oturum",
			Help:        "Aktif temayı göster veya değiştir",
			Usage:       "/theme [dark|light]",
			Description: "Tema polaritesini değiştirir; argümansız mevcutu gösterir.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				switch strings.ToLower(strings.TrimSpace(arg)) {
				case "dark":
					app.theme = DefaultTheme()
					return "tema: dark", nil, ""
				case "light":
					// Light variant: same seed lattice, lifted
					// palette. Atlas's DefaultTheme is
					// polarity-aware so re-rendering with the
					// env-resolved mode produces a light look.
					app.theme = DefaultTheme()
					return "tema: light (not: full light seed set pending — Atlas uses adaptive colors via the same DefaultTheme())", nil, ""
				default:
					return "tema: dark (light seed set deferred)", nil, ""
				}
			},
		},
		{
			Name:        "reasoning",
			Group:       "Oturum",
			Help:        "Reasoning gösterimini aç/kapat",
			Usage:       "/reasoning [on|off]",
			Description: "Reasoning pulse ve stream görünürlüğü.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "reasoning toggle (Atlas'ta thinking channel streaming yok)", nil, ""
			},
		},
		{
			Name:        "busy",
			Group:       "Oturum",
			Help:        "Tur devam ederken Enter davranışı",
			Usage:       "/busy [queue|steer|interrupt]",
			Description: "queue: mesajı kuyruğa al, drain olur · steer: next tool sonrası inject · interrupt: anında redirect.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				// Persist on the App's submission state.
				switch strings.ToLower(strings.TrimSpace(arg)) {
				case "queue":
					app.busyMode = BusyQueue
				case "steer":
					app.busyMode = BusySteer
				case "interrupt":
					app.busyMode = BusyInterrupt
				default:
					if app.busyMode == "" {
						app.busyMode = BusyQueue
					}
				}
				return "busy-mode: " + string(app.busyMode), nil, ""
			},
		},
		{
			Name:        "compress",
			Group:       "Oturum",
			Help:        "Transkripti özetleyerek kısalt",
			Usage:       "/compress [N]",
			Description: "En eski N turu tek bir özet mesajla değiştirir (context window için).",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "compress: Atlas'ta context-window sıkıştırma yok (model API'sinin native summarization'ı kullanılır)", nil, ""
			},
		},
		{
			Name:        "voice",
			Group:       "Oturum",
			Help:        "Ses kaydı için wake word'ü aç/kapat",
			Usage:       "/voice [on|off]",
			Description: "Ses wake word dinlemesini başlat/durdur.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "voice: Atlas'ta ses kaydı desteği yok", nil, ""
			},
		},
		{
			Name:        "pet",
			Group:       "Oturum",
			Help:        "Mascot animasyonunu aç/kapat",
			Usage:       "/pet [on|off]",
			Description: "Sağ alt köşedeki animasyonlu mascot.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "pet: Atlas'ta mascot yok", nil, ""
			},
		},
		{
			Name:        "subagents",
			Group:       "Oturum",
			Help:        "Subagent delegation tree'sini göster",
			Usage:       "/subagents",
			Description: "Aktif alt-agent ağacı + Gantt zaman çizelgesi.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "subagents: Atlas'ta subagent delegation yok (tek ajan)", nil, ""
			},
		},
		{
			Name:        "rollback",
			Group:       "Oturum",
			Help:        "Önceki checkpoint'e geri dön",
			Usage:       "/rollback [N]",
			Description: "N önceki tur başlangıcındaki duruma döndürür.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "rollback: Atlas'ta checkpoint yok", nil, ""
			},
		},
		{
			Name:        "replay",
			Group:       "Oturum",
			Help:        "Subagent spawn geçmişini tekrar oynat",
			Usage:       "/replay",
			Description: "Son subagent ağacı history'sini adım adım tekrar oynatır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "replay: subagent yok", nil, ""
			},
		},
		{
			Name:        "skills",
			Group:       "Oturum",
			Help:        "Yetenek (skill) listesini aç",
			Usage:       "/skills",
			Description: "Yüklü yetenekleri göster + yenisini arat.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "skills: Atlas'ta skill sistemi yok", nil, ""
			},
		},
		{
			Name:        "plugins",
			Group:       "Oturum",
			Help:        "Plugin marketplace'ini aç",
			Usage:       "/plugins",
			Description: "Yüklü plugin'leri göster, yenilerini yükle.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "plugins: Atlas'ta plugin marketplace yok", nil, ""
			},
		},
		{
			Name:        "debug-detailed",
			Group:       "Hata Ayıklama",
			Help:        "Detaylı debug trace aç/kapat",
			Usage:       "/debug-detailed [on|off]",
			Description: "Her agent event'i için tam event payload'unu loglar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				app.detailedDebug = !app.detailedDebug
				if app.detailedDebug {
					return "debug-detailed: ON", nil, ""
				}
				return "debug-detailed: OFF", nil, ""
			},
		},
	}
}

// BusyMode is the /busy policy. Mirrors Hermes's three-mode flag.
type BusyMode string

const (
	BusyQueue     BusyMode = "queue"     // append, drain after current turn
	BusySteer     BusyMode = "steer"     // inject after next tool call
	BusyInterrupt BusyMode = "interrupt" // immediately redirect
)

// quiet reference to keep tea import live in this file even when all
// commands return nil (the run signatures are typed as tea.Cmd).
var _ tea.Cmd = nil
var _ = fmt.Sprint
