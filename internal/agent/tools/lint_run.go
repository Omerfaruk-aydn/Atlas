package tools

import (
	"cmp"
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/deps/atlas-llm"
	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/lint"
)

const LintRunToolName = "lint_run"

//go:embed lint_run.md
var lintRunDescription string

const (
	defaultLintTimeout = 2 * time.Minute
	maxLintTimeout     = 15 * time.Minute
	// maxLintIssues bounds the printed list. A first run on a repository
	// that has never been linted returns thousands, and printing them
	// all buries the summary that says what kind of problem they are.
	maxLintIssues = 120
)

type LintRunParams struct {
	Dir      string   `json:"dir,omitempty" description:"A directory inside the module. Defaults to the working directory."`
	Packages string   `json:"packages,omitempty" description:"Go package pattern. Default './...'."`
	Linters  []string `json:"linters,omitempty" description:"Restrict to named checks, e.g. ['errcheck','govet']. Ignored under the vet fallback."`
	Timeout  string   `json:"timeout,omitempty" description:"How long the run may take, as a Go duration. Default 2m."`
	ForceVet *bool    `json:"force_vet,omitempty" description:"Skip golangci-lint even if installed and run go vet instead."`
}

type LintRunResponseMetadata struct {
	Issues    int    `json:"issues"`
	Tool      string `json:"tool"`
	Fallback  bool   `json:"fallback"`
	Truncated bool   `json:"truncated"`
}

func NewLintRunTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		LintRunToolName,
		lintRunDescription,
		func(ctx context.Context, params LintRunParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			timeout := defaultLintTimeout
			if params.Timeout != "" {
				parsed, err := time.ParseDuration(params.Timeout)
				if err != nil || parsed <= 0 {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("timeout %q is not a positive duration (try '90s' or '5m')", params.Timeout)), nil
				}
				timeout = min(parsed, maxLintTimeout)
			}

			dir := cmp.Or(params.Dir, workingDir)
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(workingDir, dir)
			}

			result, err := lint.Run(ctx, dir, lint.Options{
				Packages:  params.Packages,
				Timeout:   timeout,
				MaxIssues: maxLintIssues,
				Linters:   params.Linters,
				ForceVet:  params.ForceVet != nil && *params.ForceVet,
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatLint(result, params)),
				LintRunResponseMetadata{
					Issues:    len(result.Issues),
					Tool:      result.Tool,
					Fallback:  result.Fallback,
					Truncated: result.Truncated,
				},
			), nil
		},
	)
}

func formatLint(r lint.Result, params LintRunParams) string {
	var b strings.Builder

	scope := cmp.Or(params.Packages, "./...")

	if len(r.Issues) == 0 {
		fmt.Fprintf(&b, "No issues found by %s in %s.\n", r.Tool, scope)
		if r.Fallback {
			b.WriteString(writeFallbackNote())
		}
		return b.String()
	}

	fmt.Fprintf(&b, "%d issue(s) from %s in %s.\n", len(r.Issues), r.Tool, scope)
	if r.Fallback {
		b.WriteString(writeFallbackNote())
	}

	// The per-linter tally goes first: it is what says whether this is
	// one systemic pattern with one fix or many unrelated problems, and
	// that decides how to read the list below.
	counts := lint.ByLinter(r.Issues)
	if len(counts) > 1 {
		type entry struct {
			name string
			n    int
		}
		entries := make([]entry, 0, len(counts))
		for name, n := range counts {
			entries = append(entries, entry{name, n})
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].n != entries[j].n {
				return entries[i].n > entries[j].n
			}
			return entries[i].name < entries[j].name
		})
		b.WriteString("\nby check: ")
		parts := make([]string, 0, len(entries))
		for _, e := range entries {
			parts = append(parts, fmt.Sprintf("%s %d", e.name, e.n))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString("\n")
	}

	currentFile := ""
	for _, issue := range r.Issues {
		if issue.File != currentFile {
			currentFile = issue.File
			fmt.Fprintf(&b, "\n%s\n", currentFile)
		}
		if issue.Column > 0 {
			fmt.Fprintf(&b, "  %d:%d  [%s] %s\n", issue.Line, issue.Column, issue.Linter, issue.Message)
		} else {
			fmt.Fprintf(&b, "  %d  [%s] %s\n", issue.Line, issue.Linter, issue.Message)
		}
	}

	if r.Truncated {
		b.WriteString("\nThe list was cut short at the issue limit; there are more. Narrow with `packages` or `linters`.\n")
	}

	b.WriteString("\nA linter encodes opinions, and some will be wrong for a given piece of code. Read the message before changing anything, and say so if a finding is better ignored than worked around.\n")
	return b.String()
}

// writeFallbackNote spells out what the fallback does not cover. Without
// it, "no issues from go vet" reads as "this code is clean", which it is
// not evidence for.
func writeFallbackNote() string {
	return "\ngolangci-lint was not available, so go vet ran instead. vet catches printf mismatches and lock copies -- it does not check unchecked errors, unused values, or style. Treat a clean result as \"nothing obviously broken\", not as \"passes review\".\n"
}
