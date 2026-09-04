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

const GenerateDocstringToolName = "generate_docstring"

//go:embed generate_docstring.md
var generateDocstringDescription string

// maxDocstringSuggestionsShown bounds the printed list the same way the
// other codeintel tools bound theirs -- a wall of stubs for a large,
// long-undocumented package would bury the ones the caller asked about.
const maxDocstringSuggestionsShown = 40

type GenerateDocstringParams struct {
	Path         string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	Symbol       string `json:"symbol,omitempty" description:"Restrict suggestions to one exported name."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan test files. Off by default."`
}

type GenerateDocstringResponseMetadata struct {
	Total        int `json:"total"`
	FilesScanned int `json:"files_scanned"`
}

func NewGenerateDocstringTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GenerateDocstringToolName,
		generateDocstringDescription,
		func(ctx context.Context, params GenerateDocstringParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.GenerateDocstrings(root, codeintel.DocstringOptions{
				IncludeTests: params.IncludeTests != nil && *params.IncludeTests,
				Symbol:       params.Symbol,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatDocstringSuggestions(result, workingDir, params.Symbol)),
				GenerateDocstringResponseMetadata{
					Total:        len(result.Suggestions),
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatDocstringSuggestions(r codeintel.DocstringResult, workingDir, symbol string) string {
	if r.FilesScanned == 0 {
		return "No Go files found to scan. Check the path."
	}
	if len(r.Suggestions) == 0 {
		if symbol != "" {
			return fmt.Sprintf("No undocumented exported declaration named %q found.", symbol)
		}
		return fmt.Sprintf("No undocumented exported declarations found across %d Go file(s).", r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d undocumented exported declaration(s) across %d Go file(s).\n", len(r.Suggestions), r.FilesScanned)

	shown := r.Suggestions
	if len(shown) > maxDocstringSuggestionsShown {
		shown = shown[:maxDocstringSuggestionsShown]
	}

	for _, s := range shown {
		rel := relOrAbs(s.File, workingDir)
		fmt.Fprintf(&b, "\n%s:%d  %s\n%s", rel, s.Line, s.Signature, s.Stub)
	}

	if len(shown) < len(r.Suggestions) {
		fmt.Fprintf(&b, "\n... and %d more. Narrow with `path` or `symbol`.\n", len(r.Suggestions)-len(shown))
	}

	b.WriteString("\nEach stub is a shape, not prose: replace every TODO before pasting it in.\n")
	return b.String()
}
