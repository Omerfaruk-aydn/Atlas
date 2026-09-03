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

const ImpactAnalysisToolName = "impact_analysis"

//go:embed impact_analysis.md
var impactAnalysisDescription string

const (
	defaultImpactDepth = 3
	// maxImpactDepth bounds what the model can ask for. Past this the
	// result on any real repository is "most of the codebase", which
	// answers nothing and costs a lot of context.
	maxImpactDepth = 10
	// maxImpactCallers bounds the printed list.
	maxImpactCallers = 150
)

type ImpactAnalysisParams struct {
	Symbol       string `json:"symbol" description:"Function or method name to analyse. Bare name, no receiver and no package prefix."`
	Path         string `json:"path,omitempty" description:"Directory to scan. Defaults to the working directory."`
	MaxDepth     int    `json:"max_depth,omitempty" description:"How many caller hops to walk. Default 3."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan _test.go files, showing which tests cover the change. Default false."`
}

type ImpactAnalysisResponseMetadata struct {
	Callers      int  `json:"callers"`
	MaxDepth     int  `json:"max_depth"`
	Ambiguous    int  `json:"ambiguous_declarations"`
	FilesScanned int  `json:"files_scanned"`
	Truncated    bool `json:"truncated"`
}

func NewImpactAnalysisTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ImpactAnalysisToolName,
		impactAnalysisDescription,
		func(ctx context.Context, params ImpactAnalysisParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			symbol := strings.TrimSpace(params.Symbol)
			if symbol == "" {
				return fantasy.NewTextErrorResponse("symbol is required"), nil
			}
			// A qualified name is the obvious thing to pass and would
			// silently match nothing, so take the last segment rather
			// than returning an unhelpful empty result.
			if idx := strings.LastIndexAny(symbol, ".("); idx >= 0 {
				symbol = strings.Trim(symbol[idx+1:], "()")
			}

			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			depth := cmp.Or(params.MaxDepth, defaultImpactDepth)
			depth = min(depth, maxImpactDepth)

			includeTests := params.IncludeTests != nil && *params.IncludeTests

			result, err := codeintel.ImpactAnalysis(root, symbol, depth, includeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatImpact(result, symbol, workingDir)),
				ImpactAnalysisResponseMetadata{
					Callers:      len(result.Callers),
					MaxDepth:     result.MaxDepth,
					Ambiguous:    len(result.Ambiguous),
					FilesScanned: result.FilesScanned,
					Truncated:    result.Truncated,
				},
			), nil
		},
	)
}

func formatImpact(r codeintel.ImpactResult, symbol, workingDir string) string {
	if r.FilesScanned == 0 {
		return "No Go files found to scan."
	}
	if r.Target.File == "" {
		return fmt.Sprintf("No function or method named %q found in the scanned tree (%d file(s)).", symbol, r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s:%d)\n", declLabel(r.Target), relOrAbs(r.Target.File, workingDir), r.Target.Line)

	if len(r.Ambiguous) > 0 {
		fmt.Fprintf(&b, "\n%d other declaration(s) share this name, and call sites cannot be told apart without a type checker -- the callers below may belong to any of them:\n", len(r.Ambiguous))
		for _, other := range r.Ambiguous {
			fmt.Fprintf(&b, "  %s (%s:%d)\n", declLabel(other), relOrAbs(other.File, workingDir), other.Line)
		}
	}

	if len(r.Callers) == 0 {
		b.WriteString("\nNothing in this tree calls it.\n")
		b.WriteString("A call through an interface, a stored function value, or reflection would not appear here -- check type_hierarchy if it is behind an interface.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "\n%d caller(s) within %d hop(s):\n", len(r.Callers), r.MaxDepth)

	shown := r.Callers
	if len(shown) > maxImpactCallers {
		shown = shown[:maxImpactCallers]
	}

	depth := 0
	for _, c := range shown {
		if c.Depth != depth {
			depth = c.Depth
			switch depth {
			case 1:
				b.WriteString("\ndirect callers:\n")
			default:
				fmt.Fprintf(&b, "\n%d hops away:\n", depth)
			}
		}
		fmt.Fprintf(&b, "  %s (%s:%d)", declLabel(c.Func), relOrAbs(c.Func.File, workingDir), c.Func.Line)
		if c.Depth > 1 {
			fmt.Fprintf(&b, " -> %s", c.Via)
		}
		b.WriteString("\n")
	}

	if len(shown) < len(r.Callers) {
		fmt.Fprintf(&b, "\n... and %d more. Lower max_depth or narrow the path.\n", len(r.Callers)-len(shown))
	}
	if r.Truncated {
		b.WriteString("\nThe walk stopped at the depth limit: there are further callers beyond this.\n")
	}

	b.WriteString("\nCall sites are matched by name only, so this list errs wide rather than narrow. Confirm a specific one with lsp_references.\n")
	return b.String()
}

// declLabel writes a method as "(T).Name" so a method is never mistaken
// for a package-level function with the same name.
func declLabel(f codeintel.FuncRef) string {
	if f.Recv != "" {
		return fmt.Sprintf("(%s).%s", f.Recv, f.Name)
	}
	return f.Name
}
