package tools

import (
	"cmp"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/gitx"
)

const AuditTrailToolName = "audit_trail"

//go:embed audit_trail.md
var auditTrailDescription string

type AuditTrailParams struct {
	Dir    string `json:"dir,omitempty" description:"A directory inside the repository. Defaults to the working directory."`
	Path   string `json:"path" description:"The file the function is in. Required."`
	Symbol string `json:"symbol" description:"The function name. Required."`
	Limit  int    `json:"limit,omitempty" description:"How many commits to return. Omit for all of them."`
}

type AuditTrailResponseMetadata struct {
	Commits int `json:"commits"`
}

func NewAuditTrailTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		AuditTrailToolName,
		auditTrailDescription,
		func(ctx context.Context, params AuditTrailParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Path) == "" {
				return fantasy.NewTextErrorResponse("path is required"), nil
			}
			if strings.TrimSpace(params.Symbol) == "" {
				return fantasy.NewTextErrorResponse("symbol is required"), nil
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			entries, err := gitx.FunctionHistory(ctx, dir, params.Path, params.Symbol, params.Limit)
			if err != nil {
				if errors.Is(err, gitx.ErrFunctionNotFound) {
					return fantasy.NewTextResponse(fmt.Sprintf("No function named %q found in %s.", params.Symbol, params.Path)), nil
				}
				return gitErrorResponse(err), nil
			}
			if len(entries) == 0 {
				return fantasy.NewTextResponse("No history found for this function."), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatAuditTrail(entries, params.Symbol)),
				AuditTrailResponseMetadata{Commits: len(entries)},
			), nil
		},
	)
}

func formatAuditTrail(entries []gitx.FunctionHistoryEntry, symbol string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d commit(s) touched %s, most recent first.\n", len(entries), symbol)

	for _, e := range entries {
		fmt.Fprintf(&b, "\n%s  %s  %s\n%s\n", e.Short, e.Date, e.Author, e.Subject)
		if e.Diff != "" {
			fmt.Fprintf(&b, "%s\n", e.Diff)
		}
	}
	return b.String()
}
