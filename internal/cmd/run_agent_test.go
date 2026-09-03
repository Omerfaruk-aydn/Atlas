package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/stretchr/testify/require"
)

func writeRunAgentSubagent(t *testing.T, dir, name, model string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	modelLine := ""
	if model != "" {
		modelLine = "model: " + model + "\n"
	}
	content := "---\nname: " + name + "\ndescription: d\n" + modelLine + "---\n\nBody.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644))
}

func TestWrapPromptForAgentRewritesThePrompt(t *testing.T) {
	dir := t.TempDir()
	writeRunAgentSubagent(t, dir, "frontend", "")

	cfg := &config.Config{Options: &config.Options{SubagentsPaths: []string{dir}}}

	got, err := wrapPromptForAgent(cfg, "frontend", "fix the button")
	require.NoError(t, err)
	require.Contains(t, got, `agent_name="frontend"`)
	require.Contains(t, got, "fix the button")
}

func TestWrapPromptForAgentRejectsUnknownName(t *testing.T) {
	cfg := &config.Config{Options: &config.Options{}}

	_, err := wrapPromptForAgent(cfg, "nope", "do something")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no subagent named")
}

func TestWrapPromptForAgentRejectsAnUnresolvedModelRole(t *testing.T) {
	dir := t.TempDir()
	writeRunAgentSubagent(t, dir, "research", `"@research"`)

	cfg := &config.Config{Options: &config.Options{SubagentsPaths: []string{dir}}}

	_, err := wrapPromptForAgent(cfg, "research", "dig deep")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown model role")
}

func TestWrapPromptForAgentAcceptsAResolvedModelRole(t *testing.T) {
	dir := t.TempDir()
	writeRunAgentSubagent(t, dir, "research", `"@research"`)

	cfg := &config.Config{
		Options: &config.Options{
			SubagentsPaths: []string{dir},
			ModelRoles:     map[string]config.SelectedModel{"research": {Provider: "openai", Model: "o3"}},
		},
	}

	got, err := wrapPromptForAgent(cfg, "research", "dig deep")
	require.NoError(t, err)
	require.Contains(t, got, `agent_name="research"`)
}

func TestWrapPromptForAgentHandlesNilOptions(t *testing.T) {
	cfg := &config.Config{}
	_, err := wrapPromptForAgent(cfg, "nope", "do something")
	require.Error(t, err)
}
