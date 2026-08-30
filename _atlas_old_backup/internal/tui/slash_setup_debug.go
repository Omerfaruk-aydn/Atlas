package tui

import (
	"fmt"
	"runtime"
)

// setupCommands is the "Kurulum" (Setup) group: provider / model onboarding
// helpers. Currently just lists available providers, but the slot is here
// for /setup-wizard-style flows in the future.
func setupCommands(a *appForRegistry) []SlashCommand {
	return []SlashCommand{
		{
			Name:        "providers",
			Group:       "Kurulum",
			Help:        "Mevcut sağlayıcıları listele",
			Usage:       "/providers",
			Description: "Yapılandırılmış LLM sağlayıcılarını listeler.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				avail := a.Available()
				if len(avail) == 0 {
					return "(hiç sağlayıcı yok)", nil, ""
				}
				out := "Sağlayıcılar:\n"
				for _, p := range avail {
					marker := " "
					if p == a.ag.ProviderName() {
						marker = "•"
					}
					out += fmt.Sprintf("  %s %s\n", marker, p)
				}
				return out, nil, ""
			},
		},
		{
			Name:        "tools",
			Group:       "Kurulum",
			Help:        "Kayıtlı araçları listele",
			Usage:       "/tools",
			Description: "Bu oturum için kayıtlı araçların adlarını listeler.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				names := app.agent.ToolNames()
				if len(names) == 0 {
					return "(hiç araç yok)", nil, ""
				}
				out := "Araçlar:\n"
				for _, n := range names {
					out += "  · " + n + "\n"
				}
				return out, nil, ""
			},
		},
	}
}

// debugCommands is the "Hata Ayıklama" (Debug) group: process/heap info.
// Mirrors Hermes's debug.ts.
func debugCommands() []SlashCommand {
	return []SlashCommand{
		{
			Name:        "mem",
			Group:       "Hata Ayıklama",
			Help:        "Bellek kullanımını göster",
			Usage:       "/mem",
			Description: "runtime.MemStats özetini yazdırır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				return fmt.Sprintf("alloc: %s\nsys:   %s\nheap:  %s",
					fmtBytes(m.Alloc), fmtBytes(m.Sys), fmtBytes(m.HeapAlloc),
				), nil, ""
			},
		},
		{
			Name:        "theme-info",
			Group:       "Hata Ayıklama",
			Help:        "Aktif tema bilgisi",
			Usage:       "/theme-info",
			Description: "Kullanılan seed'ler ve derived token'lar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return fmt.Sprintf("accent:    %s\nprimary:   %s\nprompt:    %s\nbg:        %s",
					app.theme.Accent, app.theme.Primary, app.theme.Prompt, app.theme.HeaderBg,
				), nil, ""
			},
		},
	}
}

// fmtBytes renders 1234 as "1.2KB", 1234567 as "1.2MB".
func fmtBytes(n uint64) string {
	const (
		kb = 1024
		mb = 1024 * 1024
		gb = 1024 * 1024 * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2fGB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.2fMB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.2fKB", float64(n)/kb)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
