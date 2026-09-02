package tools

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/message"
)

//go:embed session_search.md
var sessionSearchDescription string

const SessionSearchToolName = "session_search"

type SessionSearchParams struct {
	Query     string `json:"query" description:"The words to look for. Matched literally; a trailing * matches by prefix."`
	SessionID string `json:"session_id,omitempty" description:"Limit the search to one session. Omit to search every session in this workspace."`
	Limit     int    `json:"limit,omitempty" description:"Maximum hits to return. Defaults to 20."`
}

type SessionSearchResponseMetadata struct {
	Query string              `json:"query"`
	Hits  []message.SearchHit `json:"hits"`
}

// NewSessionSearchTool lets the agent read its own history.
//
// What it returns are snippets, not whole messages: enough to tell whether a
// past exchange is the one being looked for, cheap enough to ask for
// speculatively. Following up on a hit is a matter of asking about the
// session it names.
func NewSessionSearchTool(messages message.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SessionSearchToolName,
		sessionSearchDescription,
		func(ctx context.Context, params SessionSearchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Query) == "" {
				return fantasy.NewTextErrorResponse("query is required"), nil
			}

			hits, err := messages.Search(ctx, message.SearchParams{
				Query:     params.Query,
				SessionID: params.SessionID,
				Limit:     params.Limit,
			})
			if err != nil {
				return fantasy.ToolResponse{}, fmt.Errorf("searching sessions: %w", err)
			}
			if len(hits) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf(
					"No message matches %q. Every word has to appear in the same message; try fewer, or a prefix like word*.",
					params.Query,
				)), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%d matching messages, most relevant first.\n", len(hits))
			for _, hit := range hits {
				title := hit.SessionTitle
				if title == "" {
					title = "(untitled session)"
				}
				fmt.Fprintf(&b, "\n%s in %q (session %s, %s)\n  %s\n",
					hit.Role,
					title,
					hit.SessionID,
					time.UnixMilli(hit.CreatedAt).Format(time.DateOnly),
					collapse(hit.Snippet),
				)
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(b.String()),
				SessionSearchResponseMetadata{Query: params.Query, Hits: hits},
			), nil
		},
	)
}

// collapse puts a snippet on one line. A snippet is cut from the middle of a
// message and keeps whatever newlines were there, which would otherwise
// break the one-hit-per-block shape of the result.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
