package model

import (
	"context"
	"testing"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/subagents"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/common"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/ui/dialog"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/workspace"
	"github.com/stretchr/testify/require"
)

// modelManagementWorkspace is a minimal workspace.Workspace stub
// recording SetConfigField/SaveSubagent calls and serving a fixed
// config/subagent list for reads.
type modelManagementWorkspace struct {
	workspace.Workspace
	cfg *config.Config

	setKey   string
	setValue any
	setErr   error

	subagentsList []subagents.Subagent
	savedSubagent subagents.Subagent
	savedScope    bool
	saveErr       error
}

func (w *modelManagementWorkspace) Config() *config.Config { return w.cfg }

func (w *modelManagementWorkspace) SetConfigField(_ config.Scope, key string, value any) error {
	w.setKey, w.setValue = key, value
	return w.setErr
}

func (w *modelManagementWorkspace) ListSubagents(context.Context) ([]subagents.Subagent, error) {
	return w.subagentsList, nil
}

func (w *modelManagementWorkspace) SaveSubagent(_ context.Context, sub subagents.Subagent, userScope bool) (string, error) {
	w.savedSubagent, w.savedScope = sub, userScope
	return "/agents/" + sub.Name + ".md", w.saveErr
}

func newModelManagementTestUI(ws *modelManagementWorkspace) *UI {
	return &UI{
		com:    &common.Common{Workspace: ws},
		dialog: dialog.NewOverlay(),
	}
}

func TestHandleSaveModelRoleRequiresAName(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveModelRole(dialog.ActionSaveModelRole{Args: map[string]string{"provider": "openai", "model": "gpt-4o"}})
	require.NotNil(t, cmd)
	cmd() // must not touch the workspace
	require.Empty(t, ws.setKey)
}

func TestHandleSaveModelRoleWritesTheExpectedField(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveModelRole(dialog.ActionSaveModelRole{
		Args: map[string]string{"name": "research", "provider": "openai", "model": "o3"},
	})
	require.NotNil(t, cmd)
	msg := cmd().(modelRoleSavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "options.model_roles.research", ws.setKey)
	require.Equal(t, config.SelectedModel{Provider: "openai", Model: "o3"}, ws.setValue)
}

func TestHandleSaveModelRoleEditingKeepsTheExistingName(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	// A rogue "name" in Args (the field is omitted from the edit form,
	// but nothing stops a stale value lingering) must not override
	// ExistingName.
	cmd := m.handleSaveModelRole(dialog.ActionSaveModelRole{
		ExistingName: "research",
		Args:         map[string]string{"name": "other", "provider": "openai", "model": "o3"},
	})
	msg := cmd().(modelRoleSavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "options.model_roles.research", ws.setKey)
}

func TestHandleSaveFallbackEntryAppendsToTheExistingChain(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{
		Options: &config.Options{
			ModelFallbacks: map[config.SelectedModelType][]config.SelectedModel{
				config.SelectedModelTypeLarge: {{Provider: "openai", Model: "gpt-4o-mini"}},
			},
		},
	}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveFallbackEntry(dialog.ActionSaveFallbackEntry{
		ModelType: config.SelectedModelTypeLarge,
		Args:      map[string]string{"provider": "anthropic", "model": "claude-haiku-4-5"},
	})
	require.NotNil(t, cmd)
	msg := cmd().(fallbackEntrySavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "options.model_fallbacks.large", ws.setKey)
	require.Equal(t, []config.SelectedModel{
		{Provider: "openai", Model: "gpt-4o-mini"},
		{Provider: "anthropic", Model: "claude-haiku-4-5"},
	}, ws.setValue)
}

func TestHandleSaveFallbackEntryOnAnEmptyChainStartsOne(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveFallbackEntry(dialog.ActionSaveFallbackEntry{
		ModelType: config.SelectedModelTypeSmall,
		Args:      map[string]string{"provider": "openai", "model": "gpt-4o-mini"},
	})
	msg := cmd().(fallbackEntrySavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, []config.SelectedModel{{Provider: "openai", Model: "gpt-4o-mini"}}, ws.setValue)
}

func TestHandleSaveFallbackCooldownRejectsNonNumeric(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveFallbackCooldown(dialog.ActionSaveFallbackCooldown{Args: map[string]string{"seconds": "soon"}})
	require.NotNil(t, cmd)
	cmd()
	require.Empty(t, ws.setKey, "an invalid cooldown must not reach the workspace")
}

func TestHandleSaveFallbackCooldownRejectsNegative(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveFallbackCooldown(dialog.ActionSaveFallbackCooldown{Args: map[string]string{"seconds": "-5"}})
	cmd()
	require.Empty(t, ws.setKey)
}

func TestHandleSaveFallbackCooldownWritesTheField(t *testing.T) {
	ws := &modelManagementWorkspace{cfg: &config.Config{}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveFallbackCooldown(dialog.ActionSaveFallbackCooldown{Args: map[string]string{"seconds": "300"}})
	msg := cmd().(fallbackEntrySavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "options.fallback_cooldown", ws.setKey)
	require.Equal(t, 300, ws.setValue)
}

func TestHandleSaveSubagentMetaRequiresNameAndDescription(t *testing.T) {
	ws := &modelManagementWorkspace{}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveSubagentMeta(dialog.ActionSaveSubagentMeta{Args: map[string]string{"description": "d"}})
	require.NotNil(t, cmd)
	cmd()
	require.Empty(t, ws.savedSubagent.Name, "missing name must not reach the workspace")

	cmd = m.handleSaveSubagentMeta(dialog.ActionSaveSubagentMeta{Args: map[string]string{"name": "research"}})
	cmd()
	require.Empty(t, ws.savedSubagent.Name, "missing description must not reach the workspace")
}

func TestHandleSaveSubagentMetaNewGetsThePlaceholderTemplate(t *testing.T) {
	ws := &modelManagementWorkspace{}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveSubagentMeta(dialog.ActionSaveSubagentMeta{
		UserScope: true,
		Args:      map[string]string{"name": "research", "description": "Deep research.", "model": "@research"},
	})
	msg := cmd().(subagentSavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "research", ws.savedSubagent.Name)
	require.Equal(t, "Deep research.", ws.savedSubagent.Description)
	require.Equal(t, "@research", ws.savedSubagent.Model)
	require.Contains(t, ws.savedSubagent.Instructions, "TODO")
	require.True(t, ws.savedScope)
}

// TestHandleSaveSubagentMetaEditingPreservesInstructions is the
// riskiest bit of this handler: editing a subagent's metadata must not
// silently blank out (or reset to the placeholder) instructions the
// user already wrote for it.
func TestHandleSaveSubagentMetaEditingPreservesInstructions(t *testing.T) {
	ws := &modelManagementWorkspace{subagentsList: []subagents.Subagent{
		{Name: "research", Description: "old description", Instructions: "Dig deep before answering."},
	}}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveSubagentMeta(dialog.ActionSaveSubagentMeta{
		ExistingName: "research",
		Args:         map[string]string{"description": "new description"},
	})
	msg := cmd().(subagentSavedMsg)
	require.NoError(t, msg.err)
	require.Equal(t, "research", ws.savedSubagent.Name)
	require.Equal(t, "new description", ws.savedSubagent.Description)
	require.Equal(t, "Dig deep before answering.", ws.savedSubagent.Instructions,
		"editing metadata must not touch the instructions body")
}

func TestHandleSaveSubagentMetaSaveErrorIsReported(t *testing.T) {
	ws := &modelManagementWorkspace{saveErr: context.DeadlineExceeded}
	m := newModelManagementTestUI(ws)

	cmd := m.handleSaveSubagentMeta(dialog.ActionSaveSubagentMeta{
		Args: map[string]string{"name": "research", "description": "d"},
	})
	msg := cmd().(subagentSavedMsg)
	require.Error(t, msg.err)
}
