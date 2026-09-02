package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigShowPrintsValidJSON(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"options":{"max_steps_per_turn":42}}`)

	c := newSkillTestCmd(t, runConfigShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &doc))
	options, ok := doc["options"].(map[string]any)
	require.True(t, ok, "options missing from %s", out.String())
	require.Equal(t, float64(42), options["max_steps_per_turn"])
}

func TestConfigShowRedactsProviderKeysAndMCPSecrets(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{
		"providers":{"acme":{"type":"openai","api_key":"sk-do-not-print","base_url":"https://acme.example.com"}},
		"mcp":{"srv":{"type":"stdio","command":"srv","env":{"TOKEN":"tok-do-not-print"}},
		       "web":{"type":"http","url":"https://mcp.example.com","headers":{"Authorization":"Bearer hdr-do-not-print"}}}
	}`)

	c := newSkillTestCmd(t, runConfigShow, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	got := out.String()

	require.NotContains(t, got, "sk-do-not-print")
	require.NotContains(t, got, "tok-do-not-print")
	require.NotContains(t, got, "hdr-do-not-print")

	// The shape stays readable: names and non-secret values survive, and so
	// do the env/header *keys* -- knowing TOKEN is set is the useful half.
	// Only the entries this test plants are asserted on: `config show`
	// prints the merged configuration, so whatever the machine running the
	// tests has configured globally is in there too.
	require.Contains(t, got, "https://mcp.example.com")
	require.Contains(t, got, "TOKEN")
	require.Contains(t, got, "Authorization")
	require.Contains(t, got, Redacted)
}

func TestRedactSecretsWalksNestedStructures(t *testing.T) {
	in := map[string]any{
		"keep": "visible",
		"nested": []any{
			map[string]any{"api_key": "s1", "name": "n"},
			map[string]any{"oauth": map[string]any{"access_token": "s2"}},
		},
		"env":     map[string]any{"A": "s3", "B": "s4"},
		"headers": map[string]any{"X": "s5"},
		"null_secret": map[string]any{
			"token": nil,
		},
		// Not a secret, and redacting it would hide one of the more
		// useful things in an effective config.
		"max_tokens": float64(4096),
	}

	encoded, err := json.Marshal(redactSecrets(in))
	require.NoError(t, err)
	got := string(encoded)

	for _, secret := range []string{"s1", "s2", "s3", "s4", "s5"} {
		require.NotContains(t, got, `"`+secret+`"`)
	}
	require.Contains(t, got, "visible")
	require.Contains(t, got, `"name":"n"`)
	// A secret that is not set stays null rather than reading as one that is.
	require.Contains(t, got, `"token":null`)
	require.Contains(t, got, `"max_tokens":4096`)
}

// A value that merely looks map-like under a secret-map key must still be
// redacted rather than passed through.
func TestRedactMapValuesHandlesNonMaps(t *testing.T) {
	got := redactSecrets(map[string]any{"env": []any{map[string]any{"api_key": "s"}}})
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"s"`)
}

func TestConfigPathsListsCandidatesInMergeOrder(t *testing.T) {
	workingDir := t.TempDir()
	writeAtlasConfig(t, workingDir, `{"options":{"max_steps_per_turn":1}}`)

	c := newSkillTestCmd(t, runConfigPaths, workingDir, t.TempDir())
	var out bytes.Buffer
	c.SetOut(&out)

	require.NoError(t, c.RunE(c, nil))
	got := out.String()

	require.Contains(t, got, "atlas.json")
	require.Contains(t, got, "[present]")
	require.Contains(t, got, "a setting in a later file wins")
}

func TestPrintConfigPathsMarksWhatExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "atlas.json")
	require.NoError(t, os.WriteFile(present, []byte("{}"), 0o644))
	missing := filepath.Join(dir, "nowhere", "atlas.json")

	var out bytes.Buffer
	require.NoError(t, printConfigPaths(&out, []string{missing, present}))

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Contains(t, lines[0], "[missing]")
	require.Contains(t, lines[1], "[present]")
}

// A directory where a config file could go is not a config file.
func TestPrintConfigPathsDoesNotCountADirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "atlas.json"), 0o755))

	var out bytes.Buffer
	require.NoError(t, printConfigPaths(&out, []string{filepath.Join(dir, "atlas.json")}))
	require.Contains(t, out.String(), "[missing]")
}
