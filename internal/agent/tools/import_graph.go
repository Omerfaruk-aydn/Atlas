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

const ImportGraphToolName = "import_graph"

//go:embed import_graph.md
var importGraphDescription string

// maxGraphHubs bounds the "most depended on" list in the unfocused
// summary. Dumping every package's fan-in turns the answer into a file
// listing, which the caller already has other tools for.
const maxGraphHubs = 15

type ImportGraphParams struct {
	Path         string `json:"path,omitempty" description:"Directory to scan. Defaults to the working directory."`
	Focus        string `json:"focus,omitempty" description:"A package import path or directory to detail: its imports, and everything that imports it."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan _test.go files. Default false: test-only imports may legally form cycles the compiler permits."`
}

type ImportGraphResponseMetadata struct {
	Packages int    `json:"packages"`
	Cycles   int    `json:"cycles"`
	Module   string `json:"module,omitempty"`
}

func NewImportGraphTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		ImportGraphToolName,
		importGraphDescription,
		func(ctx context.Context, params ImportGraphParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}
			includeTests := params.IncludeTests != nil && *params.IncludeTests

			result, err := codeintel.ImportGraph(root, includeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			var out string
			if params.Focus != "" {
				out = formatGraphFocus(result, params.Focus)
			} else {
				out = formatGraphSummary(result)
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(out),
				ImportGraphResponseMetadata{
					Packages: len(result.Packages),
					Cycles:   len(result.Cycles),
					Module:   result.Module,
				},
			), nil
		},
	)
}

func formatGraphSummary(r codeintel.ImportGraphResult) string {
	if len(r.Packages) == 0 {
		return "No Go packages found to scan."
	}

	var b strings.Builder
	if r.Module != "" {
		fmt.Fprintf(&b, "module %s: %d package(s).\n", r.Module, len(r.Packages))
	} else {
		fmt.Fprintf(&b, "%d package(s) (no go.mod found; paths are relative to the scan root).\n", len(r.Packages))
	}

	// Cycles first: it is the only part of this report that names a
	// build error, so it must not be scrolled past.
	if len(r.Cycles) == 0 {
		b.WriteString("No import cycles.\n")
	} else {
		fmt.Fprintf(&b, "\n%d import cycle(s) -- each package imports the next, and the last imports the first:\n", len(r.Cycles))
		for i, cycle := range r.Cycles {
			fmt.Fprintf(&b, "  %d. %s -> %s\n", i+1, strings.Join(cycle, " -> "), cycle[0])
		}
	}

	fanIn := map[string]int{}
	for _, p := range r.Packages {
		for _, dep := range p.Internal {
			fanIn[dep]++
		}
	}
	if len(fanIn) > 0 {
		type hub struct {
			path string
			n    int
		}
		hubs := make([]hub, 0, len(fanIn))
		for path, n := range fanIn {
			hubs = append(hubs, hub{path, n})
		}
		sort.Slice(hubs, func(i, j int) bool {
			if hubs[i].n != hubs[j].n {
				return hubs[i].n > hubs[j].n
			}
			return hubs[i].path < hubs[j].path
		})
		if len(hubs) > maxGraphHubs {
			hubs = hubs[:maxGraphHubs]
		}
		b.WriteString("\nMost depended on (changing these reaches the most code):\n")
		for _, h := range hubs {
			fmt.Fprintf(&b, "  %d importer(s)  %s\n", h.n, h.path)
		}
	}

	leaves := make([]string, 0, len(r.Packages))
	for _, p := range r.Packages {
		if len(p.Internal) == 0 {
			leaves = append(leaves, p.ImportPath)
		}
	}
	if len(leaves) > 0 {
		fmt.Fprintf(&b, "\n%d package(s) import nothing else in this module.\n", len(leaves))
	}

	b.WriteString("\nOnly static imports are read: a build-tagged or reflective dependency is invisible here.\n")
	return b.String()
}

// formatGraphFocus accepts either a full import path or a directory-style
// suffix, because the caller usually knows the repository-relative path
// and not the module prefix.
func formatGraphFocus(r codeintel.ImportGraphResult, focus string) string {
	focus = strings.Trim(filepath.ToSlash(focus), "/")

	var target *codeintel.PackageNode
	for i := range r.Packages {
		p := &r.Packages[i]
		if p.ImportPath == focus || strings.HasSuffix(p.ImportPath, "/"+focus) {
			target = p
			break
		}
	}
	if target == nil {
		return fmt.Sprintf("No package matching %q in the scanned tree (%d package(s)). "+
			"Give an import path or a repository-relative directory.", focus, len(r.Packages))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "package %s (%s, %d file(s))\n\n", target.ImportPath, target.Name, target.Files)

	if len(target.Internal) == 0 {
		b.WriteString("imports (in-module): none -- this is a leaf.\n")
	} else {
		fmt.Fprintf(&b, "imports (in-module), %d:\n", len(target.Internal))
		for _, dep := range target.Internal {
			fmt.Fprintf(&b, "  -> %s\n", dep)
		}
	}

	var importers []string
	for _, p := range r.Packages {
		for _, dep := range p.Internal {
			if dep == target.ImportPath {
				importers = append(importers, p.ImportPath)
				break
			}
		}
	}
	if len(importers) == 0 {
		b.WriteString("\nimported by: nothing in this module.\n")
	} else {
		fmt.Fprintf(&b, "\nimported by, %d (check these before changing the exported API):\n", len(importers))
		for _, imp := range importers {
			fmt.Fprintf(&b, "  <- %s\n", imp)
		}
	}

	if len(target.External) > 0 {
		fmt.Fprintf(&b, "\nexternal imports, %d: %s\n", len(target.External), strings.Join(target.External, ", "))
	}

	for _, cycle := range r.Cycles {
		for _, node := range cycle {
			if node == target.ImportPath {
				fmt.Fprintf(&b, "\nIn an import cycle: %s -> %s\n", strings.Join(cycle, " -> "), cycle[0])
				break
			}
		}
	}

	return b.String()
}
