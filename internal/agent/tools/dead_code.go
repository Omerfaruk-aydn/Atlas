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

const DeadCodeToolName = "dead_code"

//go:embed dead_code.md
var deadCodeDescription string

// maxDeadCodeResults bounds the report so a first scan of a large
// repository does not bury the model in thousands of lines. The count of
// what was dropped is reported alongside.
const maxDeadCodeResults = 200

type DeadCodeParams struct {
	Path              string `json:"path,omitempty" description:"Directory or single .go file to scan. Defaults to the working directory."`
	IncludeTests      *bool  `json:"include_tests,omitempty" description:"Also scan _test.go files. Default false: a symbol used only by tests is reported as dead."`
	IncludeUnexported *bool  `json:"include_unexported,omitempty" description:"Also report unexported symbols. Default true."`
}

type DeadCodeResponseMetadata struct {
	Candidates   int  `json:"candidates"`
	FilesScanned int  `json:"files_scanned"`
	Truncated    bool `json:"truncated"`
}

func NewDeadCodeTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		DeadCodeToolName,
		deadCodeDescription,
		func(ctx context.Context, params DeadCodeParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			includeTests := params.IncludeTests != nil && *params.IncludeTests
			includeUnexported := params.IncludeUnexported == nil || *params.IncludeUnexported

			result, err := codeintel.FindDeadCode(root, includeTests, includeUnexported)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			out, truncated := formatDeadCode(result, workingDir)
			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(out),
				DeadCodeResponseMetadata{
					Candidates:   len(result.Symbols),
					FilesScanned: result.FilesScanned,
					Truncated:    truncated,
				},
			), nil
		},
	)
}

func formatDeadCode(result codeintel.DeadCodeResult, workingDir string) (string, bool) {
	if result.FilesScanned == 0 {
		return "No Go files found to scan.", false
	}
	if len(result.Symbols) == 0 {
		return fmt.Sprintf("No unreferenced declarations found across %d file(s).", result.FilesScanned), false
	}

	shown := result.Symbols
	truncated := false
	if len(shown) > maxDeadCodeResults {
		shown = shown[:maxDeadCodeResults]
		truncated = true
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d unreferenced declaration(s) across %d file(s).\n",
		len(result.Symbols), result.FilesScanned)
	if result.SkippedTests {
		b.WriteString("Test files were not scanned, so symbols used only by tests appear here.\n")
	}
	b.WriteString("These are candidates to review, not safe deletions: a method satisfying an interface, or a symbol reached by reflection or code generation, is never named at its use site.\n\n")

	for _, s := range shown {
		fmt.Fprintf(&b, "%s:%d  %s %s\n", relOrAbs(s.File, workingDir), s.Line, s.Kind, s.Name)
	}
	if truncated {
		fmt.Fprintf(&b, "\n... and %d more (narrow the path to see them).\n",
			len(result.Symbols)-len(shown))
	}
	return b.String(), truncated
}

// relOrAbs shortens a path against the working directory when it sits
// inside it, and otherwise leaves it absolute so it stays unambiguous.
func relOrAbs(path, workingDir string) string {
	if workingDir == "" {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(workingDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
