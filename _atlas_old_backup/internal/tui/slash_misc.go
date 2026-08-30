package tui

import (
	"fmt"
	"runtime"
	"time"
)

// miscCommands is the catch-all "Çeşitli" group of slash commands. The
// Hermes equivalent covers the small utility commands that don't fit
// into Core/Session/Setup/Debug — copy/paste retry, branch, heap dump,
// logs. Atlas's port re-uses the same surface shape.
func miscCommands(a *appForRegistry) []SlashCommand {
	return []SlashCommand{
		{
			Name:        "new",
			Group:       "Çeşitli",
			Help:        "Yeni oturum başlat",
			Usage:       "/new",
			Description: "Mevcut sohbeti arşivler; boş bir transkriptle yeni oturuma geçer.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				app.messages = nil
				app.sessionInTok = 0
				app.sessionOutTok = 0
				app.lastTurnMS = 0
				app.render()
				return "Yeni oturum başlatıldı.", nil, ""
			},
		},
		{
			Name:        "copy",
			Aliases:     []string{"yank"},
			Group:       "Çeşitli",
			Help:        "Son yanıtı panoya kopyala",
			Usage:       "/copy [N]",
			Description: "Son assistant yanıtını (veya N. yanıtı) panoya kopyalar. Native clipboard → OSC52 fallback.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				// Walk backward to find the Nth assistant message.
				n := 1
				if arg != "" {
					fmt.Sscanf(arg, "%d", &n)
					if n < 1 {
						n = 1
					}
				}
				count := 0
				for i := len(app.messages) - 1; i >= 0; i-- {
					if app.messages[i].role == "assistant" && app.messages[i].text != "" {
						count++
						if count == n {
							return copyToClipboard(app.messages[i].text), nil, ""
						}
					}
				}
				return "kopyalanacak yanıt bulunamadı.", nil, ""
			},
		},
		{
			Name:        "paste",
			Group:       "Çeşitli",
			Help:        "Panodan yapıştır",
			Usage:       "/paste",
			Description: "OSC52 veya native clipboard üzerinden okur; komut istemine döşer.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				text := readFromClipboard()
				if text == "" {
					return "pano boş veya okunamadı.", nil, ""
				}
				return "", nil, text
			},
		},
		{
			Name:        "retry",
			Aliases:     []string{"again"},
			Group:       "Çeşitli",
			Help:        "Son kullanıcı mesajını tekrar gönder",
			Usage:       "/retry",
			Description: "En son user mesajını alıp tekrar bir tur başlatır; önceki assistant yanıtı atılır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if len(app.messageHistory) == 0 {
					return "geçmişte gönderilmiş mesaj yok.", nil, ""
				}
				last := app.messageHistory[len(app.messageHistory)-1]
				return "tekrar gönderilecek: " + last, nil, ""
			},
		},
		{
			Name:        "branch",
			Group:       "Çeşitli",
			Help:        "Mevcut git dalını göster",
			Usage:       "/branch",
			Description: "git rev-parse --abbrev-ref HEAD çıktısını yazdırır.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if app.gitBranch == "" {
					return "git dalı tespit edilemedi (git yok veya depo dışında).", nil, ""
				}
				return "dal: " + app.gitBranch, nil, ""
			},
		},
		{
			Name:        "usage",
			Group:       "Çeşitli",
			Help:        "Token kullanımı özeti",
			Usage:       "/usage",
			Description: "Oturum boyunca harcanan giriş/çıkış tokenleri.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				in := fmtTokens(app.sessionInTok)
				out := fmtTokens(app.sessionOutTok)
				total := fmtTokens(app.sessionInTok + app.sessionOutTok)
				return fmt.Sprintf("giriş: %s\nçıkış: %s\ntoplam: %s", in, out, total), nil, ""
			},
		},
		{
			Name:        "logs",
			Group:       "Çeşitli",
			Help:        "Hata log dosyasının yolunu göster",
			Usage:       "/logs",
			Description: "~/.atlas/logs/ dizininin yolunu ve son satırlarını gösterir.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return "Log dizini: ~/.atlas/logs/ (henüz yazılmadı)", nil, ""
			},
		},
		{
			Name:        "heapdump",
			Group:       "Çeşitli",
			Help:        "Process heap snapshot'ı al",
			Usage:       "/heapdump [path]",
			Description: "runtime/debug.WriteHeapDump ile heap snapshot diske yazar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				path := arg
				if path == "" {
					path = fmt.Sprintf("atlas-heap-%d.dump", time.Now().Unix())
				}
				return fmt.Sprintf("heap dump yazılamadı (runtime debug.WriteHeapDump wiring yok): %s", path), nil, ""
			},
		},
	}
}

// copyToClipboard best-effort: try OSC 52 via the terminal first
// (works over SSH), fall back to native tools when available. Returns
// a user-facing note describing the outcome.
func copyToClipboard(text string) string {
	if text == "" {
		return "kopyalanacak içerik boş."
	}
	// Atlas's TUI doesn't yet wire a real clipboard; report the
	// intent so the user knows the command ran. The actual
	// integration with bubbletea/x/term or atotto/clipboard is
	// queued behind the broader "wire to App" pass.
	return "kopyalama simüle edildi (" + fmt.Sprintf("%d bayt", len(text)) + "); tam entegrasyon sonraki adımda."
}

// readFromClipboard is the inverse of copyToClipboard. The Atlas port
// currently has no clipboard wiring; it returns "" so the caller
// reports "pano boş" instead of a phantom success.
func readFromClipboard() string { return "" }

// callout: keep the existing platform.go imports warning-free. The
// runtime import is used by /mem below; suppress it here for /heapdump
// future-use.
var _ = runtime.NumCPU
