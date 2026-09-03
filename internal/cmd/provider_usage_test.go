package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/credentials"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestProviderUsageWithNoMultiKeyProviders(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	for name := range cfg.Config().Providers.Seq2() {
		cfg.Config().Providers.Del(name)
	}
	cfg.Config().Providers.Set("single", config.ProviderConfig{ID: "single", Type: catwalk.TypeOpenAICompat, APIKey: "$KEY"})

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listProviderUsage(c, cfg, ""))
	require.Contains(t, out.String(), "nothing to rotate")
}

func TestProviderUsageShowsTheNextPick(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	cfg.Config().Options.DataDirectory = dataDir
	for name := range cfg.Config().Providers.Seq2() {
		cfg.Config().Providers.Del(name)
	}
	cfg.Config().Providers.Set("multi", config.ProviderConfig{
		ID: "multi", Type: catwalk.TypeOpenAICompat, APIKey: "$KEY1", APIKeys: []string{"$KEY2", "$KEY3"},
	})

	statePath := filepath.Join(dataDir, credentials.StateFileName)
	require.NoError(t, os.WriteFile(statePath, []byte(`{"next":{"multi":1}}`), 0o644))

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listProviderUsage(c, cfg, ""))
	require.Contains(t, out.String(), "multi: 3 keys, next pick is key #2 of 3")
}

func TestProviderUsageFiltersByName(t *testing.T) {
	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	cfg.Config().Options.DataDirectory = dataDir
	for name := range cfg.Config().Providers.Seq2() {
		cfg.Config().Providers.Del(name)
	}
	cfg.Config().Providers.Set("multi", config.ProviderConfig{
		ID: "multi", Type: catwalk.TypeOpenAICompat, APIKey: "$KEY1", APIKeys: []string{"$KEY2"},
	})
	cfg.Config().Providers.Set("other", config.ProviderConfig{
		ID: "other", Type: catwalk.TypeOpenAICompat, APIKey: "$KEY1", APIKeys: []string{"$KEY2"},
	})

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listProviderUsage(c, cfg, "multi"))
	got := out.String()
	require.Contains(t, got, "multi:")
	require.NotContains(t, got, "other:")
}

func TestProviderUsageJSON(t *testing.T) {
	providerUsageJSON = true
	t.Cleanup(func() { providerUsageJSON = false })

	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	cfg.Config().Options.DataDirectory = dataDir
	for name := range cfg.Config().Providers.Seq2() {
		cfg.Config().Providers.Del(name)
	}
	cfg.Config().Providers.Set("multi", config.ProviderConfig{
		ID: "multi", Type: catwalk.TypeOpenAICompat, APIKey: "$KEY1", APIKeys: []string{"$KEY2"},
	})

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listProviderUsage(c, cfg, ""))

	var got []jsonProviderUsage
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, []jsonProviderUsage{{Name: "multi", Keys: 2, NextKeyIdx: 0, NextKeyOf: 2}}, got)
}
