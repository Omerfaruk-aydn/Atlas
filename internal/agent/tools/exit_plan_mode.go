package tools

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/maincodss/atlas-agent/internal/deps/fantasy"
	"github.com/maincodss/atlas-agent/internal/permission"
)

//go:embed exit_plan_mode.md
var exitPlanModeDescription string

// ExitPlanModeToolName must match permission.ExitPlanModeToolName: the
// permission service special-cases this exact tool name so it can still
// reach the confirmation dialog while ModePlan denies everything else
// outright.
const ExitPlanModeToolName = permission.ExitPlanModeToolName

type ExitPlanModeParams struct {
	Plan string `json:"plan" description:"The implementation plan you just presented to the user, in markdown. Shown again in the approval prompt."`
}

func NewExitPlanModeTool(permissions permission.Service, workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ExitPlanModeToolName,
		exitPlanModeDescription,
		func(ctx context.Context, params ExitPlanModeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if permissions.Mode() != permission.ModePlan {
				return fantasy.NewTextResponse("Not in plan mode; nothing to exit."), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session_id is required")
			}

			description := params.Plan
			if description == "" {
				description = "Exit plan mode and start implementing."
			}

			granted, err := permissions.Request(ctx, permission.CreatePermissionRequest{
				SessionID:   sessionID,
				Path:        workingDir,
				ToolCallID:  call.ID,
				ToolName:    ExitPlanModeToolName,
				Action:      "exit_plan_mode",
				Description: description,
			})
			if err != nil {
				return fantasy.ToolResponse{}, err
			}
			if !granted {
				return fantasy.NewTextResponse("The user did not approve the plan. Stay in plan mode: keep discussing or refining the plan as text, and do not retry write/edit/bash tools."), nil
			}

			// The dialog approval IS the user's mode switch here — flipping
			// back to ModeManual (not ModeBypass) so the tools this plan
			// needs still go through their normal confirmation, same as if
			// the user had pressed Shift+Tab themselves.
			permissions.SetMode(permission.ModeManual)
			return fantasy.NewTextResponse("Plan approved. Plan mode is now off (switched to manual mode); proceed with the implementation."), nil
		},
	)
}
