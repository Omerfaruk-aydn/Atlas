package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	cfg, err := Load(filepath.Join(dir, "nonexistent.toml"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("expected default provider anthropic, got %s", cfg.DefaultProvider)
	}
	if len(cfg.Providers) != 3 {
		t.Errorf("expected 3 default providers, got %d", len(cfg.Providers))
	}
}

func TestLoadProjectOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	projectCfg := filepath.Join(dir, "atlas.toml")
	content := `default_provider = "openai"
auto_approve = true
`
	if err := os.WriteFile(projectCfg, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(projectCfg)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("expected overridden provider openai, got %s", cfg.DefaultProvider)
	}
	if !cfg.AutoApprove {
		t.Errorf("expected auto_approve true from project override")
	}
	// defaults for providers should still be present since project file didn't redefine them
	if len(cfg.Providers) != 3 {
		t.Errorf("expected default providers to remain, got %d", len(cfg.Providers))
	}
}

func TestResolveAPIKey(t *testing.T) {
	cfg := Defaults()
	t.Setenv("ANTHROPIC_API_KEY", "test-key-123")

	key, err := cfg.ResolveAPIKey("anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "test-key-123" {
		t.Errorf("expected test-key-123, got %s", key)
	}

	if _, err := cfg.ResolveAPIKey("unknown"); err == nil {
		t.Error("expected error for unknown provider")
	}
}
