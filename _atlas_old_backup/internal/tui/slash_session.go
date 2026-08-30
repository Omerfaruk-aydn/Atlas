package tui

import "fmt"

func sessionCommands(a *appForRegistry) []SlashCommand {
	return []SlashCommand{
		{
			Name:        "tokens",
			Group:       "Oturum",
			Help:        "Oturum token kullanımını göster",
			Usage:       "/tokens",
			Description: "Giriş/çıkış token toplamı.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return fmt.Sprintf("tokens %d↑ %d↓", app.sessionInTok, app.sessionOutTok), nil, ""
			},
		},
		{
			Name:        "history",
			Group:       "Oturum",
			Help:        "Gönderilen mesajları göster",
			Usage:       "/history",
			Description: "Bu oturumda gönderilen tüm kullanıcı mesajları.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if len(app.messageHistory) == 0 {
					return "(geçmiş boş)", nil, ""
				}
				out := "Geçmiş:\n"
				for i, m := range app.messageHistory {
					out += "  " + fmt.Sprintf("%d. %s", i+1, truncateForNote(m)) + "\n"
				}
				return out, nil, ""
			},
		},
		{
			Name:        "queue",
			Group:       "Oturum",
			Help:        "Kuyruktaki mesajları göster",
			Usage:       "/queue",
			Description: "Şu anda tur sonu bekleyen mesajlar.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				if len(app.queuedMessages) == 0 {
					return "(kuyruk boş)", nil, ""
				}
				out := "Kuyruk:\n"
				for i, m := range app.queuedMessages {
					out += "  " + fmt.Sprintf("%d. %s", i+1, truncateForNote(m)) + "\n"
				}
				return out, nil, ""
			},
		},
		{
			Name:        "status",
			Group:       "Oturum",
			Help:        "Mevcut oturum durumunu göster",
			Usage:       "/status",
			Description: "Sağlayıcı, model, token, son tur süresi.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				return fmt.Sprintf("sağlayıcı: %s\nmodel: %s\ntokens: %d↑ %d↓\nson tur: %.1fs",
					app.agent.ProviderName(),
					app.agent.CurrentModel(),
					app.sessionInTok, app.sessionOutTok,
					float64(app.lastTurnMS)/1000,
				), nil, ""
			},
		},
		{
			Name:        "details",
			Group:       "Oturum",
			Help:        "Araç/akıl yürütme detaylarını aç/kapat",
			Usage:       "/details [hidden|collapsed|expanded]",
			Description: "Transkript'te reasoning/tools/activity görünürlüğünü kontrol eder.",
			Run: func(arg string, app *App) (string, teaCmd, string) {
				switch arg {
				case "hidden":
					app.details.Global = DetailsHidden
				case "collapsed":
					app.details.Global = DetailsCollapsed
				case "expanded":
					app.details.Global = DetailsExpanded
				default:
					app.details.Global = nextDetailsMode(app.details.Global)
				}
				return fmt.Sprintf("details: %s", detailsModeName(app.details.Global)), nil, ""
			},
		},
	}
}

// detailsModeName renders a DetailsMode as a one-word label.
func detailsModeName(d DetailsMode) string {
	switch d {
	case DetailsHidden:
		return "hidden"
	case DetailsCollapsed:
		return "collapsed"
	default:
		return "expanded"
	}
}
