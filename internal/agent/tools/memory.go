package tools

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/memory"
)

//go:embed memory.md
var memoryDescription string

const MemoryToolName = "memory"

type MemoryParams struct {
	Action string `json:"action" description:"add, replace, remove, or set"`
	Scope  string `json:"scope" description:"project for facts about this codebase, user for facts about the person"`
	Entry  string `json:"entry,omitempty" description:"For add: the single-line entry to record. For set: the full new contents of the store."`
	Old    string `json:"old,omitempty" description:"For replace and remove: the existing text to act on. Must appear exactly once."`
	New    string `json:"new,omitempty" description:"For replace: the text to put in its place."`
}

type MemoryResponseMetadata struct {
	Action  string `json:"action"`
	Scope   string `json:"scope"`
	Content string `json:"content"`
	Used    int    `json:"used"`
	Limit   int    `json:"limit"`
}

// NewMemoryTool exposes the persistent stores to the agent.
//
// Everything it writes lands on disk immediately but is not re-read into the
// running conversation: the stores are loaded once, when a session starts.
// That is deliberate and is explained where they are loaded -- rewriting the
// system prompt mid-session would throw away the provider's prefix cache for
// a fact that can wait until the next session.
func NewMemoryTool(store *memory.Store) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		MemoryToolName,
		memoryDescription,
		func(ctx context.Context, params MemoryParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			scope := memory.Scope(strings.TrimSpace(params.Scope))
			if !scope.Valid() {
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"unknown scope %q: use %q for facts about this codebase or %q for facts about the person",
					params.Scope, memory.ScopeProject, memory.ScopeUser,
				)), nil
			}

			var (
				content string
				err     error
				action  = strings.TrimSpace(strings.ToLower(params.Action))
			)
			switch action {
			case "add":
				content, err = store.Add(scope, params.Entry)
			case "replace":
				content, err = store.Replace(scope, params.Old, params.New)
			case "remove":
				content, err = store.Remove(scope, params.Old)
			case "set":
				content, err = store.Set(scope, params.Entry)
			default:
				return fantasy.NewTextErrorResponse(fmt.Sprintf(
					"unknown action %q: use add, replace, remove, or set", params.Action,
				)), nil
			}

			// These are all things the model can fix by trying again
			// differently -- a longer excerpt, a shorter store -- so they
			// come back as a tool result it can read rather than as an error
			// that ends the turn.
			var tooLong *memory.ErrTooLong
			switch {
			case errors.As(err, &tooLong):
				return fantasy.NewTextErrorResponse(tooLong.Error()), nil
			case errors.Is(err, memory.ErrNotFound), errors.Is(err, memory.ErrAmbiguous):
				return fantasy.NewTextErrorResponse(err.Error()), nil
			case err != nil:
				return fantasy.ToolResponse{}, err
			}

			used, limit, err := store.Used(scope)
			if err != nil {
				return fantasy.ToolResponse{}, err
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%s memory updated (%s).\n", scope, action)
			if limit >= 0 {
				fmt.Fprintf(&b, "Using %d of %d characters.\n", used, limit)
			}
			b.WriteString("\nIt now reads:\n\n")
			if strings.TrimSpace(content) == "" {
				b.WriteString("(empty)\n")
			} else {
				b.WriteString(content)
			}
			b.WriteString("\nThis takes effect in the next session; it is not re-read into the current one.")

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(b.String()),
				MemoryResponseMetadata{
					Action:  action,
					Scope:   string(scope),
					Content: content,
					Used:    used,
					Limit:   limit,
				},
			), nil
		},
	)
}
