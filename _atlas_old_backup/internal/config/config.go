package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type ProviderConfig struct {
	Name      string `toml:"name"`
	Model     string `toml:"model"`
	APIKeyEnv string `toml:"api_key_env"`
	BaseURL   string `toml:"base_url,omitempty"`
}

type MCPServerConfig struct {
	Name    string   `toml:"name"`
	Enabled bool     `toml:"enabled"`
	Command string   `toml:"command,omitempty"`
	Args    []string `toml:"args,omitempty"`
	URL     string   `toml:"url,omitempty"`
}

type Config struct {
	DefaultProvider string            `toml:"default_provider"`
	AutoApprove     bool              `toml:"auto_approve"`
	Providers       []ProviderConfig  `toml:"providers"`
	MCPServers      []MCPServerConfig `toml:"mcp_servers"`
}

func Defaults() Config {
	return Config{
		DefaultProvider: "anthropic",
		AutoApprove:     false,
		Providers: []ProviderConfig{
			{Name: "anthropic", Model: "claude-sonnet-4-5", APIKeyEnv: "ANTHROPIC_API_KEY"},
			{Name: "openai", Model: "gpt-5", APIKeyEnv: "OPENAI_API_KEY"},
			{Name: "gemini", Model: "gemini-2.5-pro", APIKeyEnv: "GOOGLE_API_KEY"},
		},
	}
}

// Load reads config from (in order of precedence, lowest to highest):
// built-in defaults -> user config (~/.atlas/config.toml) -> project config (./atlas.toml).
// Environment variables referenced by APIKeyEnv are resolved at use time, not here.
func Load(projectConfigPath string) (Config, error) {
	cfg := Defaults()

	home, err := os.UserHomeDir()
	if err == nil {
		userPath := filepath.Join(home, ".atlas", "config.toml")
		if err := mergeFile(&cfg, userPath); err != nil {
			return cfg, err
		}
	}

	if projectConfigPath == "" {
		projectConfigPath = "atlas.toml"
	}
	if err := mergeFile(&cfg, projectConfigPath); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading config %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing config %s: %w", path, err)
	}
	return nil
}

// ResolveAPIKey returns the API key for a provider by reading its configured
// environment variable.
func (c Config) ResolveAPIKey(providerName string) (string, error) {
	for _, p := range c.Providers {
		if p.Name != providerName {
			continue
		}
		key := os.Getenv(p.APIKeyEnv)
		if key == "" {
			return "", fmt.Errorf("environment variable %s is not set for provider %s", p.APIKeyEnv, providerName)
		}
		return key, nil
	}
	return "", fmt.Errorf("unknown provider %q", providerName)
}

// Provider returns the configuration for a named provider.
func (c Config) Provider(name string) (ProviderConfig, bool) {
	for _, p := range c.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return ProviderConfig{}, false
}
