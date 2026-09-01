// Package main is the entry point for the Atlas-Agent CLI.
//
//	@title			ATLAS-AGENT API
//	@version		1.0
//	@description	ATLAS-AGENT is a terminal-based AI coding assistant. This API is served over a Unix socket (or Windows named pipe) and provides programmatic access to workspaces, sessions, agents, LSP, MCP, and more.
//	@contact.name	Atlas
//	@contact.url	https://charm.sh
//	@license.name	MIT
//	@license.url	https://github.com/maincodss/atlas-agent/blob/main/LICENSE
//	@BasePath		/v1
package main

import (
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/maincodss/atlas-agent/internal/cmd"
	_ "github.com/maincodss/atlas-agent/internal/dns"
	_ "github.com/joho/godotenv/autoload"
)

func main() {
	if os.Getenv("ATLAS-AGENT_PROFILE") != "" {
		go func() {
			slog.Info("Serving pprof at localhost:6060")
			if httpErr := http.ListenAndServe("localhost:6060", nil); httpErr != nil {
				slog.Error("Failed to pprof listen", "error", httpErr)
			}
		}()
	}

	cmd.Execute()
}
