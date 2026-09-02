package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
	"github.com/stretchr/testify/require"
)

func newSkillTool(t *testing.T, builtins ...*skills.Skill) (fantasy.AgentTool, SkillDirs) {
	t.Helper()
	dir := t.TempDir()
	dirs := SkillDirs{
		Project: filepath.Join(dir, "project"),
		User:    filepath.Join(dir, "user"),
	}
	return NewSkillManageTool(dirs, builtins, permission.NewPermissionService(t.TempDir(), true, nil)), dirs
}

func runSkill(t *testing.T, tool fantasy.AgentTool, params SkillManageParams) fantasy.ToolResponse {
	t.Helper()

	input, err := json.Marshal(params)
	require.NoError(t, err)

	res, err := tool.Run(
		context.WithValue(t.Context(), SessionIDContextKey, "test-session"),
		fantasy.ToolCall{ID: "test-call", Name: SkillManageToolName, Input: string(input)},
	)
	require.NoError(t, err)
	return res
}

func TestSkillManageWritesADiscoverableSkill(t *testing.T) {
	t.Parallel()
	tool, dirs := newSkillTool(t)

	res := runSkill(t, tool, SkillManageParams{
		Action:       "create",
		Scope:        "project",
		Name:         "release-checklist",
		Description:  "Use when cutting a release.",
		Instructions: "Run the tests before tagging.",
	})

	require.False(t, res.IsError)
	require.Contains(t, res.Content, "next session")

	found := skills.Discover([]string{dirs.Project})
	require.Len(t, found, 1)
	require.Equal(t, "release-checklist", found[0].Name)
	require.Equal(t, "Run the tests before tagging.", found[0].Instructions)
}

func TestSkillManageWontOverwriteBlindly(t *testing.T) {
	t.Parallel()
	tool, _ := newSkillTool(t)

	params := SkillManageParams{
		Action: "create", Scope: "user", Name: "a-skill",
		Description: "d", Instructions: "b",
	}
	require.False(t, runSkill(t, tool, params).IsError)

	res := runSkill(t, tool, params)
	require.True(t, res.IsError)
	require.Contains(t, res.Content, "use update to rewrite it")
}

func TestSkillManageUpdateNeedsSomethingToUpdate(t *testing.T) {
	t.Parallel()
	tool, _ := newSkillTool(t)

	res := runSkill(t, tool, SkillManageParams{
		Action: "update", Scope: "project", Name: "never-written",
		Description: "d", Instructions: "b",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "use create to write a new one")
}

func TestSkillManageRefusesToShadowABuiltin(t *testing.T) {
	t.Parallel()
	tool, dirs := newSkillTool(t, &skills.Skill{Name: "atlas-config", Builtin: true})

	res := runSkill(t, tool, SkillManageParams{
		Action: "create", Scope: "project", Name: "atlas-config",
		Description: "d", Instructions: "b",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "built in")
	require.NoDirExists(t, filepath.Join(dirs.Project, "atlas-config"))
}

func TestSkillManageInsistsOnADescription(t *testing.T) {
	t.Parallel()
	tool, _ := newSkillTool(t)

	res := runSkill(t, tool, SkillManageParams{
		Action: "create", Scope: "project", Name: "no-description", Instructions: "b",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "before deciding to load the skill")
}

func TestSkillManageRejectsAnUnknownScope(t *testing.T) {
	t.Parallel()
	tool, _ := newSkillTool(t)

	res := runSkill(t, tool, SkillManageParams{
		Action: "create", Scope: "everywhere", Name: "x", Description: "d", Instructions: "b",
	})

	require.True(t, res.IsError)
	require.Contains(t, res.Content, "unknown scope")
}

func TestSkillManageDelete(t *testing.T) {
	t.Parallel()
	tool, dirs := newSkillTool(t)

	require.False(t, runSkill(t, tool, SkillManageParams{
		Action: "create", Scope: "project", Name: "short-lived",
		Description: "d", Instructions: "b",
	}).IsError)

	res := runSkill(t, tool, SkillManageParams{Action: "delete", Scope: "project", Name: "short-lived"})
	require.False(t, res.IsError)
	require.NoDirExists(t, filepath.Join(dirs.Project, "short-lived"))
}

func TestSkillManageAsksBeforeWriting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dirs := SkillDirs{Project: filepath.Join(dir, "project"), User: filepath.Join(dir, "user")}
	permissions := permission.NewPermissionService(t.TempDir(), false, nil)
	tool := NewSkillManageTool(dirs, nil, permissions)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	events := permissions.Subscribe(ctx)

	var (
		mu   sync.Mutex
		seen []permission.PermissionRequest
		done = make(chan struct{})
	)
	go func() {
		defer close(done)
		for event := range events {
			mu.Lock()
			seen = append(seen, event.Payload)
			mu.Unlock()
			permissions.Deny(event.Payload)
		}
	}()

	input, err := json.Marshal(SkillManageParams{
		Action: "create", Scope: "project", Name: "denied-skill",
		Description: "d", Instructions: "b",
	})
	require.NoError(t, err)

	res, err := tool.Run(context.WithValue(ctx, SessionIDContextKey, "s"), fantasy.ToolCall{
		ID: "c", Name: SkillManageToolName, Input: string(input),
	})
	require.NoError(t, err)
	cancel()
	<-done

	require.True(t, res.IsError)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, seen, 1)
	params, ok := seen[0].Params.(SkillManagePermissionParams)
	require.True(t, ok)
	require.Empty(t, params.OldContent)
	require.Contains(t, params.NewContent, "denied-skill")

	_, statErr := os.Stat(filepath.Join(dirs.Project, "denied-skill"))
	require.True(t, os.IsNotExist(statErr), "denied means nothing on disk")
}
