package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/codeintel"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
)

const SecurityScanToolName = "security_scan"

//go:embed security_scan.md
var securityScanDescription string

// maxSecurityFindingsShown bounds the printed list the same way
// anti_pattern_scan bounds its own: the breakdown by kind is the useful
// summary, and a wall of hundreds of lines buries it.
const maxSecurityFindingsShown = 80

type SecurityScanParams struct {
	Path         string `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan test files. Off by default."`
}

type SecurityScanResponseMetadata struct {
	Total        int            `json:"total"`
	ByKind       map[string]int `json:"by_kind"`
	FilesScanned int            `json:"files_scanned"`
}

func NewSecurityScanTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		SecurityScanToolName,
		securityScanDescription,
		func(ctx context.Context, params SecurityScanParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.SecurityScan(root, codeintel.SecurityScanOptions{
				IncludeTests: params.IncludeTests != nil && *params.IncludeTests,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatSecurityFindings(result, workingDir)),
				SecurityScanResponseMetadata{
					Total:        len(result.Findings),
					ByKind:       result.ByKind,
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

func formatSecurityFindings(r codeintel.SecurityScanResult, workingDir string) string {
	if r.FilesScanned == 0 {
		return "No Go files found to scan. Check the path."
	}
	if len(r.Findings) == 0 {
		return fmt.Sprintf("No security smells found across %d Go file(s).", r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d finding(s) across %d Go file(s).\n", len(r.Findings), r.FilesScanned)

	if len(r.ByKind) > 0 {
		type entry struct {
			kind string
			n    int
		}
		entries := make([]entry, 0, len(r.ByKind))
		for kind, n := range r.ByKind {
			entries = append(entries, entry{kind, n})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].n != entries[j].n {
				return entries[i].n > entries[j].n
			}
			return entries[i].kind < entries[j].kind
		})
		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			parts = append(parts, fmt.Sprintf("%s %d", e.kind, e.n))
		}
		fmt.Fprintf(&b, "by kind: %s\n", strings.Join(parts, ", "))
	}

	shown := r.Findings
	if len(shown) > maxSecurityFindingsShown {
		shown = shown[:maxSecurityFindingsShown]
	}

	currentFile := ""
	for _, f := range shown {
		rel := relOrAbs(f.File, workingDir)
		if rel != currentFile {
			currentFile = rel
			fmt.Fprintf(&b, "\n%s\n", rel)
		}
		fmt.Fprintf(&b, "  %d  [%s] %s\n", f.Line, f.Kind, f.Message)
	}

	if len(shown) < len(r.Findings) {
		fmt.Fprintf(&b, "\n... and %d more. Narrow with `path`.\n", len(r.Findings)-len(shown))
	}

	b.WriteString("\nThis is name- and shape-based, not type-checked or data-flow aware: confirm exploitability before treating a finding as a real vulnerability.\n")
	return b.String()
}
