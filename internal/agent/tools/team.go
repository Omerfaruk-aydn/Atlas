package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/teams"
)

const (
	TeamSendToolName = "team_send"
	TeamReadToolName = "team_read"
)

//go:embed team_send.md
var teamSendDescription string

//go:embed team_read.md
var teamReadDescription string

// maxTeamWaitSeconds caps team_read's wait_seconds so a single call cannot
// block a turn indefinitely.
const maxTeamWaitSeconds = 60

// teamRegistry is the seam the team tools depend on instead of
// *teams.Registry directly, so tests can supply a fake without relying on
// the real registry's timing behavior.
type teamRegistry interface {
	TeamFor(sessionID string) string
	Send(teamID, from, text string) teams.Message
	Wait(ctx context.Context, teamID string, since int, timeout time.Duration) []teams.Message
}

type TeamSendParams struct {
	Text string `json:"text" description:"Message to broadcast to every agent in this task's team: the session that started the task, plus every sub-agent it has spawned, however deeply nested."`
}

type TeamReadParams struct {
	Since       int `json:"since,omitempty" description:"Only return messages with a sequence number greater than this. Pass the seq_after value a previous team_send/team_read call returned to avoid re-reading old messages."`
	WaitSeconds int `json:"wait_seconds,omitempty" description:"If nothing is available yet, wait up to this many seconds (max 60) for a message to arrive instead of returning empty right away. Defaults to 0: return immediately."`
}

type TeamResponseMetadata struct {
	Action   string `json:"action"`
	SeqAfter int    `json:"seq_after"`
}

func NewTeamSendTool(registry teamRegistry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeamSendToolName,
		teamSendDescription,
		func(ctx context.Context, params TeamSendParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			text := strings.TrimSpace(params.Text)
			if text == "" {
				return fantasy.NewTextErrorResponse("text is required"), nil
			}

			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for team_send")
			}

			teamID := registry.TeamFor(sessionID)
			msg := registry.Send(teamID, sessionID, text)

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(fmt.Sprintf("Sent (seq %d). Use team_read with since=%d to see replies.", msg.Seq, msg.Seq)),
				TeamResponseMetadata{Action: "send", SeqAfter: msg.Seq},
			), nil
		},
	)
}

func NewTeamReadTool(registry teamRegistry) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TeamReadToolName,
		teamReadDescription,
		func(ctx context.Context, params TeamReadParams, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			sessionID := GetSessionFromContext(ctx)
			if sessionID == "" {
				return fantasy.ToolResponse{}, fmt.Errorf("session ID is required for team_read")
			}
			if params.WaitSeconds < 0 {
				return fantasy.NewTextErrorResponse("wait_seconds must not be negative"), nil
			}

			wait := params.WaitSeconds
			if wait > maxTeamWaitSeconds {
				wait = maxTeamWaitSeconds
			}

			teamID := registry.TeamFor(sessionID)
			msgs := registry.Wait(ctx, teamID, params.Since, time.Duration(wait)*time.Second)

			seqAfter := params.Since
			if len(msgs) > 0 {
				seqAfter = msgs[len(msgs)-1].Seq
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatTeamMessages(msgs)),
				TeamResponseMetadata{Action: "read", SeqAfter: seqAfter},
			), nil
		},
	)
}

func formatTeamMessages(msgs []teams.Message) string {
	if len(msgs) == 0 {
		return "No new team messages."
	}
	var b strings.Builder
	for _, m := range msgs {
		fmt.Fprintf(&b, "[seq %d] %s (%s): %s\n", m.Seq, m.From, m.Time.Format(time.RFC3339), m.Text)
	}
	return strings.TrimRight(b.String(), "\n")
}
