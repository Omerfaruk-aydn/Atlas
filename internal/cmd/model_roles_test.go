package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestListModelRolesWithNoneConfigured(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)
	cfg.Config().Models = nil

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listModelRoles(c, cfg))
	require.Contains(t, out.String(), "No model roles configured.")
}

func TestListModelRolesShowsLargeSmallAndCustom(t *testing.T) {
	cfg, err := config.Init(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)
	cfg.Config().Models = map[config.SelectedModelType]config.SelectedModel{
		config.SelectedModelTypeLarge: {Provider: "openai", Model: "gpt-5"},
		config.SelectedModelTypeSmall: {Provider: "openai", Model: "gpt-5-mini"},
	}
	cfg.Config().Options.ModelRoles = map[string]config.SelectedModel{
		"research": {Provider: "openai", Model: "o3"},
	}

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listModelRoles(c, cfg))
	got := out.String()
	require.Contains(t, got, "large: openai/gpt-5")
	require.Contains(t, got, "small: openai/gpt-5-mini")
	require.Contains(t, got, "research: openai/o3")
}

func TestListModelRolesJSON(t *testing.T) {
	modelRolesJSON = true
	t.Cleanup(func() { modelRolesJSON = false })

	cfg, err := config.Init(t.TempDir(), t.TempDir(), false)
	require.NoError(t, err)
	cfg.Config().Models = map[config.SelectedModelType]config.SelectedModel{
		config.SelectedModelTypeLarge: {Provider: "openai", Model: "gpt-5"},
	}

	c := &cobra.Command{}
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, listModelRoles(c, cfg))

	var got []jsonModelRole
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))

	var found bool
	for _, r := range got {
		if r.Role == "large" {
			found = true
			require.Equal(t, "openai", r.Provider)
			require.Equal(t, "gpt-5", r.Model)
		}
	}
	require.True(t, found)
}
