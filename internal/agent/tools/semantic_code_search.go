package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/codeintel"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const SemanticCodeSearchToolName = "semantic_code_search"

//go:embed semantic_code_search.md
var semanticCodeSearchDescription string

type SemanticCodeSearchParams struct {
	Path         string `json:"path,omitempty" description:"Directory or single file to search. Defaults to the working directory."`
	Query        string `json:"query" description:"Natural-language description of what to find. Required."`
	Limit        int    `json:"limit,omitempty" description:"Maximum number of results. Defaults to 10."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also search test files. Off by default."`
}

type SemanticCodeSearchResponseMetadata struct {
	Matches      int `json:"matches"`
	FilesScanned int `json:"files_scanned"`
}

func NewSemanticCodeSearchTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SemanticCodeSearchToolName,
		semanticCodeSearchDescription,
		func(ctx context.Context, params SemanticCodeSearchParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Query) == "" {
				return fantasy.NewTextErrorResponse("query is required"), nil
			}

			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.SemanticSearch(root, params.Query, codeintel.SearchOptions{
				IncludeTests: params.IncludeTests != nil && *params.IncludeTests,
				Limit:        params.Limit,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.FilesScanned == 0 {
				return fantasy.NewTextResponse("No Go files found to search. Check the path."), nil
			}
			if len(result.Matches) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf(
					"No symbol's name or doc comment shares vocabulary with %q, across %d Go file(s). Try different words, or grep for a specific string instead.",
					params.Query, result.FilesScanned)), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatSemanticSearch(result, workingDir)),
				SemanticCodeSearchResponseMetadata{
					Matches:      len(result.Matches),
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatSemanticSearch(r codeintel.SearchResult, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) across %d Go file(s).\n", len(r.Matches), r.FilesScanned)

	for _, m := range r.Matches {
		rel := relOrAbs(m.File, workingDir)
		fmt.Fprintf(&b, "\n%s  %s:%d  (score %d, matched: %s)\n", m.Signature, rel, m.Line, m.Score, strings.Join(m.MatchedTerms, ", "))
		if doc := firstLine(m.Doc); doc != "" {
			fmt.Fprintf(&b, "  %s\n", doc)
		}
	}
	return b.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
