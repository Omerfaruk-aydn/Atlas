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

const GenerateTestsToolName = "generate_tests"

//go:embed generate_tests.md
var generateTestsDescription string

type GenerateTestsParams struct {
	Path   string `json:"path,omitempty" description:"Directory or single file to search. Defaults to the working directory."`
	Symbol string `json:"symbol" description:"The function name to generate a test for. Required."`
}

type GenerateTestsResponseMetadata struct {
	FuncName string   `json:"func_name"`
	Imports  []string `json:"imports"`
}

func NewGenerateTestsTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		GenerateTestsToolName,
		generateTestsDescription,
		func(ctx context.Context, params GenerateTestsParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if strings.TrimSpace(params.Symbol) == "" {
				return fantasy.NewTextErrorResponse("symbol is required"), nil
			}

			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			skeleton, err := codeintel.GenerateTestSkeleton(root, params.Symbol)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatTestSkeleton(skeleton, workingDir)),
				GenerateTestsResponseMetadata{
					FuncName: skeleton.FuncName,
					Imports:  skeleton.Imports,
				},
			), nil
		},
	)
}

func formatTestSkeleton(s codeintel.TestSkeleton, workingDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s at %s:%d\n\n", s.FuncName, relOrAbs(s.File, workingDir), s.Line)
	fmt.Fprintf(&b, "Needs these imports if not already present: %s\n\n", strings.Join(s.Imports, ", "))
	b.WriteString(s.Skeleton)
	b.WriteString("\nThis has one placeholder case -- add real inputs and expectations before it tests anything.\n")
	return b.String()
}
