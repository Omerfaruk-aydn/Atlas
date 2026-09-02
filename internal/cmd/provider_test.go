package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-models/pkg/catwalk"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func newProviderTestCmd(t *testing.T, dataDir string) *cobra.Command {
	t.Helper()
	c := &cobra.Command{RunE: runProviderTest}
	c.Flags().String("data-dir", dataDir, "")
	c.Flags().Bool("debug", false, "")
	c.SetContext(t.Context())
	return c
}

// TestProviderTestUnknownProvider exercises runProviderTest's own logic:
// resolving a name against the loaded config and reporting clearly when it
// is not there. It needs no provider configured and no network access.
func TestProviderTestUnknownProvider(t *testing.T) {
	dataDir := t.TempDir()
	c := newProviderTestCmd(t, dataDir)

	err := c.RunE(c, []string{"no-such-provider"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "no provider named")
}

// The rest of runProviderTest is three lines of glue over
// ProviderConfig.TestConnection, which is pre-existing and does the actual
// network round-trip; these tests exercise that call directly against a
// local server rather than re-deriving CLI-level fixtures for it.

func TestProviderConnectionSucceedsOn200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	providerCfg := config.ProviderConfig{
		ID:      "local",
		Type:    catwalk.TypeOpenAICompat,
		BaseURL: server.URL,
		APIKey:  "test-key",
	}

	require.NoError(t, providerCfg.TestConnection(config.IdentityResolver()))
}

func TestProviderConnectionFailsOn401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	providerCfg := config.ProviderConfig{
		ID:      "local",
		Type:    catwalk.TypeOpenAICompat,
		BaseURL: server.URL,
		APIKey:  "wrong-key",
	}

	require.Error(t, providerCfg.TestConnection(config.IdentityResolver()))
}

func TestProviderTestAllReportsEachOne(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ok.Close)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(bad.Close)

	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	cfg.Config().Providers.Set("good", config.ProviderConfig{ID: "good", Type: catwalk.TypeOpenAICompat, BaseURL: ok.URL, APIKey: "k"})
	cfg.Config().Providers.Set("bad", config.ProviderConfig{ID: "bad", Type: catwalk.TypeOpenAICompat, BaseURL: bad.URL, APIKey: "k"})
	cfg.Config().Providers.Set("disabled", config.ProviderConfig{ID: "disabled", Type: catwalk.TypeOpenAICompat, BaseURL: bad.URL, Disable: true})

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	err = testAllProviders(c, cfg)

	require.Error(t, err, "one of the two enabled providers failed, so the run as a whole failed")
	require.Contains(t, out.String(), "good: ok")
	require.Contains(t, out.String(), "bad: failed")
	require.NotContains(t, out.String(), "disabled", "a disabled provider is not tested")
}

func TestProviderTestAllSucceedsWhenEveryoneIsHealthy(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ok.Close)

	dataDir := t.TempDir()
	cfg, err := config.Init(dataDir, dataDir, false)
	require.NoError(t, err)
	cfg.Config().Providers.Set("good", config.ProviderConfig{ID: "good", Type: catwalk.TypeOpenAICompat, BaseURL: ok.URL, APIKey: "k"})

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, testAllProviders(c, cfg))
}
