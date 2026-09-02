package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/home"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/skills"
)

//go:embed skill_manage.md
var skillManageDescription string

const SkillManageToolName = "skill_manage"

type SkillManageParams struct {
	Action       string `json:"action" description:"create, update, patch, or delete"`
	Scope        string `json:"scope" description:"project to write into this repository, user to write into the user's config directory"`
	Name         string `json:"name" description:"Lowercase words joined by hyphens. Also the directory name."`
	Description  string `json:"description,omitempty" description:"When a future session should reach for this skill. Required for create and update."`
	Instructions string `json:"instructions,omitempty" description:"The body of the skill, in markdown. Required for create and update."`
	Old          string `json:"old,omitempty" description:"For patch: the existing text to change. Must appear exactly once in the skill's instructions."`
	New          string `json:"new,omitempty" description:"For patch: the text to put in its place."`
}

// SkillManagePermissionParams is what the approval dialog is given.
type SkillManagePermissionParams struct {
	FilePath   string `json:"file_path"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
}

type SkillManageResponseMetadata struct {
	Action string `json:"action"`
	Scope  string `json:"scope"`
	Name   string `json:"name"`
	Path   string `json:"path"`
}

// SkillDirs says where each scope writes.
type SkillDirs struct {
	// Project is the skills directory inside the repository.
	Project string
	// User is the skills directory in the user's config directory.
	User string
}

func (d SkillDirs) dir(scope string) (string, error) {
	switch scope {
	case "project":
		return d.Project, nil
	case "user":
		return d.User, nil
	}
	return "", fmt.Errorf("unknown scope %q: use project to write into this repository, or user to write into the user's config directory", scope)
}

// NewSkillManageTool lets the agent write down a procedure for its successors.
//
// It writes only into the two directories this workspace already discovers,
// and never over a builtin: a skill that shadowed a builtin by editing it
// would be invisible to anyone reading the repository, and impossible to
// undo by deleting a file.
//
// Like memory, a skill written now is read at the start of the next session.
// The list of available skills is part of the system prompt, and rewriting
// that mid-session would throw away the provider's cache of it.
func NewSkillManageTool(dirs SkillDirs, builtins []*skills.Skill, permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SkillManageToolName,
		skillManageDescription,
		func(ctx context.Context, params SkillManageParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			name := strings.TrimSpace(strings.ToLower(params.Name))
			if name == "" {
				return fantasy.NewTextErrorResponse("name is required"), nil
			}

			dir, err := dirs.dir(strings.TrimSpace(strings.ToLower(params.Scope)))
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			if s, ok := skills.Find(builtins, name); ok && s.Builtin {
				return fantasy.NewTextErrorResponse(skills.ErrBuiltin.Error()), nil
			}

			path := skills.SkillPath(dir, name)
			existing, existed := readIfPresent(path)

			action := strings.TrimSpace(strings.ToLower(params.Action))
			var next string
			switch action {
			case "patch":
				if !existed {
					return fantasy.NewTextErrorResponse(fmt.Sprintf(
						"no skill named %q in %s", name, home.Short(dir),
					)), nil
				}
				current, err := skills.ParseContent([]byte(existing))
				if err != nil {
					return fantasy.ToolResponse{}, fmt.Errorf("the skill on disk no longer parses: %w", err)
				}
				patched, err := skills.Patch(current, params.Old, params.New)
				switch {
				case errors.Is(err, skills.ErrPatchNotFound), errors.Is(err, skills.ErrPatchAmbiguous):
					return fantasy.NewTextErrorResponse(err.Error()), nil
				case err != nil:
					return fantasy.ToolResponse{}, err
				}
				rendered, err := skills.Render(patched)
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				next = string(rendered)

			case "create", "update":
				if action == "create" && existed {
					return fantasy.NewTextErrorResponse(fmt.Sprintf(
						"a skill named %q already exists at %s; use update to rewrite it",
						name, home.Short(path),
					)), nil
				}
				if action == "update" && !existed {
					return fantasy.NewTextErrorResponse(fmt.Sprintf(
						"no skill named %q in %s; use create to write a new one", name, home.Short(dir),
					)), nil
				}
				if strings.TrimSpace(params.Description) == "" {
					return fantasy.NewTextErrorResponse("description is required: it is the only thing a future session sees before deciding to load the skill"), nil
				}
				if strings.TrimSpace(params.Instructions) == "" {
					return fantasy.NewTextErrorResponse("instructions is required"), nil
				}

				rendered, err := skills.Render(&skills.Skill{
					Name:         name,
					Description:  strings.TrimSpace(params.Description),
					Instructions: params.Instructions,
				})
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				next = string(rendered)

			case "delete":
				if !existed {
					return fantasy.NewTextErrorResponse(fmt.Sprintf(
						"no skill named %q in %s", name, home.Short(dir),
					)), nil
				}
				next = ""

			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"unknown action %q: use create, update, patch, or delete", params.Action,
				)), nil
			}

			if next == existing {
				return fantasy.NewTextResponse(fmt.Sprintf(
					"%s already reads exactly that; nothing written.", home.Short(path),
				)), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, errors.New("session_id is required")
			}
			granted, err := permissions.Request(ctx, permission.CreatePermissionRequest{
				SessionID:   sessionID,
				Path:        path,
				ToolCallID:  call.ID,
				ToolName:    SkillManageToolName,
				Action:      action,
				Description: fmt.Sprintf("%s skill %q (%s)", action, name, home.Short(path)),
				Params: SkillManagePermissionParams{
					FilePath:   path,
					OldContent: existing,
					NewContent: next,
				},
			})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return NewPermissionDeniedResponse(permissions), nil
			}

			if action == "delete" {
				if err := skills.Delete(dir, name); err != nil {
					return fantasy.ToolResponse{}, err
				}
			} else {
				// next is already the fully rendered file -- from create,
				// update, or patch -- so parse it back rather than
				// re-deriving a Skill from params, which patch does not
				// populate. Save validates before writing: a skill file
				// that does not parse is worse than none, since it shows
				// up in the discovery diagnostics of every later session.
				toSave, err := skills.ParseContent([]byte(next))
				if err != nil {
					return fantasy.ToolResponse{}, err
				}
				toSave.Name = name
				if _, err := skills.Save(dir, toSave); err != nil {
					return fantasy.NewTextErrorResponse(err.Error()), nil
				}
			}

			verb := map[string]string{"create": "Wrote", "update": "Rewrote", "patch": "Patched", "delete": "Deleted"}[action]
			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(fmt.Sprintf(
					"%s %s.\nIt takes effect in the next session; the skill list in this one is already fixed.",
					verb, home.Short(path),
				)),
				SkillManageResponseMetadata{Action: action, Scope: params.Scope, Name: name, Path: path},
			), nil
		},
	)
}

func readIfPresent(path string) (content string, existed bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}
