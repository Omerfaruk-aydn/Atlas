package agent

import (
	"path/filepath"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/agent/tools"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/config"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
)

// memoryStore builds the store for a workspace.
//
// Project memory goes in the project's data directory, beside the session
// database, so it is scoped to the code it describes and travels with a
// checkout that keeps that directory. The user profile goes in the global
// config directory, because it describes the person rather than any one
// project.
func memoryStore(cfg *config.ConfigStore) *memory.Store {
	opts := memory.Options{
		ProjectDir: filepath.Join(cfg.Config().Options.DataDirectory, "memory"),
		UserDir:    filepath.Join(home.Config(), "atlas"),
	}
	if m := cfg.Config().Options.Memory; m != nil {
		opts.ProjectLimit = m.ProjectLimit
		opts.UserLimit = m.UserLimit
	}
	return memory.New(opts)
}

// skillDirs says where a skill the agent writes should go: into the
// repository for something about this code, or into the user's config
// directory for something about how they work. Both are already on the
// discovery path, so a skill written to either is found without any
// configuration.
func skillDirs(cfg *config.ConfigStore) tools.SkillDirs {
	return tools.SkillDirs{
		Project: filepath.Join(cfg.WorkingDir(), ".atlas", "skills"),
		User:    filepath.Join(home.Config(), "atlas", "skills"),
	}
}
