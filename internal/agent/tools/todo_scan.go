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

const TodoScanToolName = "todo_scan"

//go:embed todo_scan.md
var todoScanDescription string

const (
	defaultTodoMaxResults = 500
	// maxTodosShown bounds the printed list. A mature codebase has
	// hundreds, and printing them all replaces the summary -- which is
	// the part that says what kind of debt this is -- with a wall.
	maxTodosShown = 80
)

type TodoScanParams struct {
	Path         string   `json:"path,omitempty" description:"Directory or single file to scan. Defaults to the working directory."`
	Kinds        []string `json:"kinds,omitempty" description:"Restrict to specific markers, e.g. ['FIXME','BUG']."`
	Extensions   []string `json:"extensions,omitempty" description:"Restrict to file types, e.g. ['go','ts']."`
	IncludeTests *bool    `json:"include_tests,omitempty" description:"Also scan test files. Off by default."`
	MaxResults   int      `json:"max_results,omitempty" description:"Cap the number of findings. Default 500."`
}

type TodoScanResponseMetadata struct {
	Total        int            `json:"total"`
	ByKind       map[string]int `json:"by_kind"`
	Owned        int            `json:"owned"`
	Ticketed     int            `json:"ticketed"`
	FilesScanned int            `json:"files_scanned"`
	Truncated    bool           `json:"truncated"`
}

func NewTodoScanTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TodoScanToolName,
		todoScanDescription,
		func(ctx context.Context, params TodoScanParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.FindTodos(root, codeintel.TodoOptions{
				Kinds:        params.Kinds,
				Extensions:   params.Extensions,
				IncludeTests: params.IncludeTests != nil && *params.IncludeTests,
				MaxResults:   cmp.Or(params.MaxResults, defaultTodoMaxResults),
			})
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			owned, ticketed := 0, 0
			for _, todo := range result.Todos {
				if todo.Owner != "" {
					owned++
				}
				if todo.Ticket != "" {
					ticketed++
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatTodos(result, workingDir, owned, ticketed)),
				TodoScanResponseMetadata{
					Total:        len(result.Todos),
					ByKind:       result.ByKind,
					Owned:        owned,
					Ticketed:     ticketed,
					FilesScanned: result.FilesScanned,
					Truncated:    result.Truncated,
				},
			), nil
		},
	)
}

func formatTodos(r codeintel.TodoResult, workingDir string, owned, ticketed int) string {
	if r.FilesScanned == 0 {
		return "No scannable files found. Check the path, or pass `extensions` if this project uses file types the scanner does not know."
	}
	if len(r.Todos) == 0 {
		return fmt.Sprintf("No markers found across %d file(s).", r.FilesScanned)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d marker(s) across %d file(s).\n", len(r.Todos), r.FilesScanned)

	// The breakdown leads. A raw total says nothing; the mix of kinds is
	// what distinguishes a codebase with aspirational TODOs from one
	// carrying recorded defects.
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
	fmt.Fprintf(&b, "%d have an owner, %d reference a ticket.\n", owned, ticketed)

	shown := r.Todos
	if len(shown) > maxTodosShown {
		shown = shown[:maxTodosShown]
	}

	currentFile := ""
	for _, todo := range shown {
		rel := relOrAbs(todo.File, workingDir)
		if rel != currentFile {
			currentFile = rel
			fmt.Fprintf(&b, "\n%s\n", rel)
		}
		fmt.Fprintf(&b, "  %d  %s", todo.Line, todo.Kind)
		if todo.Owner != "" {
			fmt.Fprintf(&b, "(%s)", todo.Owner)
		}
		if todo.Ticket != "" {
			fmt.Fprintf(&b, " [%s]", todo.Ticket)
		}
		if todo.Text != "" {
			fmt.Fprintf(&b, ": %s", truncateText(todo.Text, 160))
		}
		b.WriteString("\n")
	}

	if len(shown) < len(r.Todos) {
		fmt.Fprintf(&b, "\n... and %d more. Narrow with `kinds` or `path`.\n", len(r.Todos)-len(shown))
	}
	if r.Truncated {
		b.WriteString("\nThe scan stopped at its result limit, so there may be more beyond these.\n")
	}

	b.WriteString("\nA count is not a task list: many markers are years old, already done, or notes to nobody. FIXME and BUG usually record a known defect; TODO is often aspirational.\n")
	return b.String()
}

func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
