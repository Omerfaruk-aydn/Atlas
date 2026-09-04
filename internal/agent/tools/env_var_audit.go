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

const EnvVarAuditToolName = "env_var_audit"

//go:embed env_var_audit.md
var envVarAuditDescription string

type EnvVarAuditParams struct {
	Path         string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan test files. Off by default."`
}

type EnvVarAuditResponseMetadata struct {
	Total        int  `json:"total"`
	Undocumented int  `json:"undocumented"`
	EnvFileFound bool `json:"env_file_found"`
	FilesScanned int  `json:"files_scanned"`
}

func NewEnvVarAuditTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		EnvVarAuditToolName,
		envVarAuditDescription,
		func(ctx context.Context, params EnvVarAuditParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.AuditEnvVars(root, params.IncludeTests != nil && *params.IncludeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			if result.FilesScanned == 0 {
				return fantasy.NewTextResponse("No Go files found to scan. Check the path."), nil
			}
			if len(result.Usages) == 0 {
				return fantasy.NewTextResponse(fmt.Sprintf("No os.Getenv/LookupEnv/Setenv calls found across %d Go file(s).", result.FilesScanned)), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatEnvAudit(result, workingDir)),
				EnvVarAuditResponseMetadata{
					Total:        len(result.Usages),
					Undocumented: len(result.Undocumented),
					EnvFileFound: result.EnvFileFound,
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatEnvAudit(r codeintel.EnvAuditResult, workingDir string) string {
	var b strings.Builder

	names := map[string]bool{}
	for _, u := range r.Usages {
		names[u.Name] = true
	}
	fmt.Fprintf(&b, "%d distinct variable(s), %d usage(s), across %d Go file(s).\n", len(names), len(r.Usages), r.FilesScanned)

	currentFile := ""
	for _, u := range r.Usages {
		rel := relOrAbs(u.File, workingDir)
		if rel != currentFile {
			currentFile = rel
			fmt.Fprintf(&b, "\n%s\n", rel)
		}
		fmt.Fprintf(&b, "  %d  [%s] %s\n", u.Line, u.Kind, u.Name)
	}

	if !r.EnvFileFound {
		b.WriteString("\nNo .env.example, .env.sample, or .env.dist found -- documentation coverage was not checked.\n")
	} else if len(r.Undocumented) > 0 {
		fmt.Fprintf(&b, "\nUndocumented (read in code, missing from %s): %s\n", relOrAbs(r.EnvFilePath, workingDir), strings.Join(r.Undocumented, ", "))
	} else {
		fmt.Fprintf(&b, "\nEvery variable read is documented in %s.\n", relOrAbs(r.EnvFilePath, workingDir))
	}
	return b.String()
}
