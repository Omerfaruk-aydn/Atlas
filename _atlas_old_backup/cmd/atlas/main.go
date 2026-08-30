package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/omerfarukaydin/atlas/internal/agent"
	"github.com/omerfarukaydin/atlas/internal/config"
	"github.com/omerfarukaydin/atlas/internal/llm"

	_ "github.com/omerfarukaydin/atlas/internal/llm/anthropic"
	_ "github.com/omerfarukaydin/atlas/internal/llm/gemini"
	_ "github.com/omerfarukaydin/atlas/internal/llm/openai"

	"github.com/omerfarukaydin/atlas/internal/tools"
	"github.com/omerfarukaydin/atlas/internal/tools/file"
	"github.com/omerfarukaydin/atlas/internal/tools/shell"
	"github.com/omerfarukaydin/atlas/internal/tools/web"

	"github.com/omerfarukaydin/atlas/internal/tui"
	"github.com/spf13/cobra"
)

var configPath string

const systemPrompt = "Sen Atlas, terminalde çalışan yardımcı bir AI asistansın. Kısa ve net cevaplar ver. " +
	"Dosya okuma/yazma/düzenleme, kabuk komutu çalıştırma ve URL getirme araçların var; gerektiğinde kullan."

func newToolRegistry() *tools.Registry {
	reg := tools.NewRegistry()
	reg.Register(file.ReadTool{})
	reg.Register(file.WriteTool{})
	reg.Register(file.EditTool{})
	reg.Register(shell.ExecTool{})
	reg.Register(web.FetchTool{})
	return reg
}

func main() {
	root := &cobra.Command{
		Use:           "atlas",
		Short:         "Atlas — çoklu sağlayıcılı terminal AI agent",
		RunE:          runChat,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "proje config dosyası yolu (varsayılan: ./atlas.toml)")

	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Yüklenen konfigürasyonu göster",
		RunE:  runConfig,
	}
	root.AddCommand(configCmd)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	fmt.Printf("Atlas — varsayılan sağlayıcı: %s\n", cfg.DefaultProvider)
	for _, p := range cfg.Providers {
		fmt.Printf("  - %-10s model=%-20s api_key_env=%s\n", p.Name, p.Model, p.APIKeyEnv)
	}
	fmt.Printf("auto_approve: %v\n", cfg.AutoApprove)
	fmt.Printf("mcp_servers: %d tanımlı\n", len(cfg.MCPServers))
	return nil
}

// buildProvider resolves a named provider's config + API key and constructs
// it via the llm registry.
func buildProvider(cfg config.Config, name string) (llm.Provider, error) {
	pc, ok := cfg.Provider(name)
	if !ok {
		return nil, fmt.Errorf("sağlayıcı bulunamadı: %s (mevcut: %v)", name, llm.Available())
	}
	apiKey, err := cfg.ResolveAPIKey(name)
	if err != nil {
		return nil, fmt.Errorf("%w\nİpucu: %s ortam değişkenini ayarla", err, pc.APIKeyEnv)
	}
	return llm.New(name, apiKey, pc.Model)
}

func runChat(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	provider, err := buildProvider(cfg, cfg.DefaultProvider)
	if err != nil {
		return err
	}
	ag := agent.New(provider, systemPrompt, newToolRegistry(), cfg.AutoApprove)

	switcher := func(name string) (llm.Provider, error) {
		return buildProvider(cfg, name)
	}

	p := tea.NewProgram(tui.New(ag, switcher), tea.WithAltScreen())
	_, err = p.Run()
	return err
}
