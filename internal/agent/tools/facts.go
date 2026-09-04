package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/factstore"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/permission"
)

const FactsToolName = "facts"

//go:embed facts.md
var factsDescription string

type FactsParams struct {
	Action string   `json:"action" description:"retain, recall, or reflect"`
	Text   string   `json:"text,omitempty" description:"For retain: the fact to remember."`
	Tags   []string `json:"tags,omitempty" description:"For retain: optional labels to tag this fact with."`
	Query  string   `json:"query,omitempty" description:"For recall: words to search retained facts for. Empty returns the most recent facts."`
	Limit  int      `json:"limit,omitempty" description:"For recall: maximum results. Defaults to 10."`
}

// FactsPermissionParams is what the approval dialog is given for a
// retain call.
type FactsPermissionParams struct {
	FilePath string   `json:"file_path"`
	Text     string   `json:"text"`
	Tags     []string `json:"tags,omitempty"`
}

type FactsResponseMetadata struct {
	Action  string `json:"action"`
	Total   int    `json:"total,omitempty"`
	Matches int    `json:"matches,omitempty"`
}

// NewFactsTool exposes a queryable, in-session fact store -- the gap
// `memory` deliberately leaves open by only being read once at session
// start. See facts.md for the distinction.
//
// Only retain writes to disk, and only retain asks permission first, the
// same reasoning memory.go uses: a write that persists past this tool
// call and is not re-read into the conversation automatically deserves
// one question, the same as an edit to a file would.
func NewFactsTool(store *factstore.Store, permissions permission.Service) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		FactsToolName,
		factsDescription,
		func(ctx context.Context, params FactsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			switch strings.ToLower(strings.TrimSpace(params.Action)) {
			case "retain":
				return retainFact(ctx, store, permissions, call, params)
			case "recall":
				return recallFacts(store, params)
			case "reflect":
				return reflectFacts(store)
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"unknown action %q: use retain, recall, or reflect", params.Action,
				)), nil
			}
		},
	)
}

func retainFact(ctx context.Context, store *factstore.Store, permissions permission.Service, call fantasy.ToolCall, params FactsParams) (fantasy.ToolResponse, error) {
	if strings.TrimSpace(params.Text) == "" {
		return fantasy.NewTextErrorResponse("text is required for retain"), nil
	}

	sessionID := GetSessionFromContext(ctx)
	if sessionID == "" {
		return fantasy.ToolResponse{}, errors.New("session_id is required")
	}

	granted, err := permissions.Request(ctx, permission.CreatePermissionRequest{
		SessionID:   sessionID,
		Path:        store.Path(),
		ToolCallID:  call.ID,
		ToolName:    FactsToolName,
		Action:      "retain",
		Description: "Retain a fact for this session",
		Params: FactsPermissionParams{
			FilePath: store.Path(),
			Text:     params.Text,
			Tags:     params.Tags,
		},
	})
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if !granted {
		return NewPermissionDeniedResponse(permissions), nil
	}

	fact, err := store.Retain(params.Text, params.Tags)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}

	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(fmt.Sprintf("Retained (id %s). Recall it later with `recall` -- it will not reappear on its own.", fact.ID)),
		FactsResponseMetadata{Action: "retain"},
	), nil
}

func recallFacts(store *factstore.Store, params FactsParams) (fantasy.ToolResponse, error) {
	results, err := store.Recall(params.Query, params.Limit)
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if len(results) == 0 {
		if strings.TrimSpace(params.Query) == "" {
			return fantasy.NewTextResponse("Nothing has been retained yet."), nil
		}
		return fantasy.NewTextResponse(fmt.Sprintf("No retained fact matches %q.", params.Query)), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d fact(s).\n", len(results))
	for _, r := range results {
		fmt.Fprintf(&b, "\n[%s] %s\n", r.ID, r.Text)
		if len(r.Tags) > 0 {
			fmt.Fprintf(&b, "  tags: %s\n", strings.Join(r.Tags, ", "))
		}
	}

	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(b.String()),
		FactsResponseMetadata{Action: "recall", Matches: len(results)},
	), nil
}

func reflectFacts(store *factstore.Store) (fantasy.ToolResponse, error) {
	result, err := store.Reflect()
	if err != nil {
		return fantasy.ToolResponse{}, err
	}
	if result.Total == 0 {
		return fantasy.NewTextResponse("Nothing has been retained yet."), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d fact(s) retained, from %s to %s.\n", result.Total,
		result.OldestAt.Format("2006-01-02 15:04"), result.NewestAt.Format("2006-01-02 15:04"))

	if len(result.ByTag) > 0 {
		b.WriteString("\nBy tag:\n")
		for tag, n := range result.ByTag {
			fmt.Fprintf(&b, "  %s: %d\n", tag, n)
		}
	}

	if len(result.Duplicates) > 0 {
		fmt.Fprintf(&b, "\n%d group(s) of near-duplicate facts:\n", len(result.Duplicates))
		for _, group := range result.Duplicates {
			fmt.Fprintf(&b, "  %q, retained %d times\n", group[0].Text, len(group))
		}
	}

	return fantasy.WithResponseMetadata(
		fantasy.NewTextResponse(b.String()),
		FactsResponseMetadata{Action: "reflect", Total: result.Total},
	), nil
}
