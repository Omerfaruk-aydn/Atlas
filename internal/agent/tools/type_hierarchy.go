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

const TypeHierarchyToolName = "type_hierarchy"

//go:embed type_hierarchy.md
var typeHierarchyDescription string

// maxHierarchyInterfaces bounds an unfocused report. A repository with
// hundreds of interfaces produces a wall of text nobody reads; the count
// of what was dropped is reported, with a pointer at `focus`.
const maxHierarchyInterfaces = 40

type TypeHierarchyParams struct {
	Path         string `json:"path,omitempty" description:"Directory to scan. Defaults to the working directory."`
	Focus        string `json:"focus,omitempty" description:"An interface or type name to narrow the report to. Omit for every interface in the tree."`
	IncludeTests *bool  `json:"include_tests,omitempty" description:"Also scan _test.go files, which finds test fakes and mocks. Default false."`
}

type TypeHierarchyResponseMetadata struct {
	Interfaces      int  `json:"interfaces"`
	Types           int  `json:"types"`
	Implementations int  `json:"implementations"`
	FilesScanned    int  `json:"files_scanned"`
	Truncated       bool `json:"truncated"`
}

func NewTypeHierarchyTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		TypeHierarchyToolName,
		typeHierarchyDescription,
		func(ctx context.Context, params TypeHierarchyParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}
			includeTests := params.IncludeTests != nil && *params.IncludeTests

			result, err := codeintel.TypeHierarchy(root, includeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			out, truncated := formatHierarchy(result, params.Focus, workingDir)
			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(out),
				TypeHierarchyResponseMetadata{
					Interfaces:      len(result.Interfaces),
					Types:           len(result.Types),
					Implementations: len(result.Implementations),
					FilesScanned:    result.FilesScanned,
					Truncated:       truncated,
				},
			), nil
		},
	)
}

func formatHierarchy(r codeintel.HierarchyResult, focus, workingDir string) (string, bool) {
	if r.FilesScanned == 0 {
		return "No Go files found to scan.", false
	}
	if focus != "" {
		return formatHierarchyFocus(r, focus, workingDir), false
	}
	return formatHierarchyAll(r, workingDir)
}

// formatHierarchyFocus answers about one name, which may be either side
// of the relation. Checking both is what makes a single `focus`
// parameter workable instead of forcing the caller to know in advance
// whether the name is an interface.
func formatHierarchyFocus(r codeintel.HierarchyResult, focus, workingDir string) string {
	var b strings.Builder

	for _, iface := range r.Interfaces {
		if iface.Name != focus {
			continue
		}
		fmt.Fprintf(&b, "interface %s (%s:%d)\n", iface.Name, relOrAbs(iface.File, workingDir), iface.Line)
		for _, m := range iface.Methods {
			fmt.Fprintf(&b, "  %s%s\n", m.Name, m.Sig)
		}
		if len(iface.Embeds) > 0 {
			fmt.Fprintf(&b, "  embeds: %s\n", strings.Join(iface.Embeds, ", "))
		}
		b.WriteString("\nimplemented by:\n")
		found := false
		for _, impl := range r.Implementations {
			if impl.Interface.Name != focus {
				continue
			}
			found = true
			fmt.Fprintf(&b, "  %s (%s:%d)\n", implName(impl),
				relOrAbs(impl.Type.File, workingDir), impl.Type.Line)
		}
		if !found {
			b.WriteString("  (nothing in this tree -- note that implementations in other modules are invisible here)\n")
		}
		return b.String()
	}

	for _, ct := range r.Types {
		if ct.Name != focus {
			continue
		}
		fmt.Fprintf(&b, "type %s (%s:%d)\n", ct.Name, relOrAbs(ct.File, workingDir), ct.Line)
		for _, m := range ct.Methods {
			fmt.Fprintf(&b, "  %s%s\n", m.Name, m.Sig)
		}
		b.WriteString("\nsatisfies:\n")
		found := false
		for _, impl := range r.Implementations {
			if impl.Type.Name != focus {
				continue
			}
			found = true
			fmt.Fprintf(&b, "  %s (%s:%d)\n", impl.Interface.Name,
				relOrAbs(impl.Interface.File, workingDir), impl.Interface.Line)
		}
		if !found {
			b.WriteString("  (no interface in this tree)\n")
		}
		return b.String()
	}

	return fmt.Sprintf("No interface or type named %q found in the scanned tree (%d file(s)).", focus, r.FilesScanned)
}

func formatHierarchyAll(r codeintel.HierarchyResult, workingDir string) (string, bool) {
	// Only interfaces with at least one implementation are worth listing
	// unfocused; the rest are noise the caller can reach via focus.
	byIface := map[string][]codeintel.Implementation{}
	for _, impl := range r.Implementations {
		byIface[impl.Interface.Name] = append(byIface[impl.Interface.Name], impl)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d interface(s), %d named type(s), %d implementation(s) across %d file(s).\n",
		len(r.Interfaces), len(r.Types), len(r.Implementations), r.FilesScanned)
	b.WriteString("Interfaces from outside this tree are invisible, so an absent match does not prove one does not exist.\n\n")

	shown, truncated := 0, false
	for _, iface := range r.Interfaces {
		impls := byIface[iface.Name]
		if len(impls) == 0 {
			continue
		}
		if shown >= maxHierarchyInterfaces {
			truncated = true
			break
		}
		shown++
		fmt.Fprintf(&b, "%s (%s:%d) <- ", iface.Name, relOrAbs(iface.File, workingDir), iface.Line)
		parts := make([]string, 0, len(impls))
		for _, impl := range impls {
			parts = append(parts, implName(impl))
		}
		b.WriteString(strings.Join(parts, ", "))
		b.WriteString("\n")
	}

	if shown == 0 {
		b.WriteString("No interface in this tree is implemented by a type in this tree.\n")
	}
	if truncated {
		fmt.Fprintf(&b, "\n... more interfaces not shown. Use focus=<name> for one of them.\n")
	}
	return b.String(), truncated
}

// implName spells the type as it must be written at an assignment: a
// pointer-receiver method set belongs to *T, not T, and getting that
// wrong is a compile error the caller would otherwise walk into.
func implName(impl codeintel.Implementation) string {
	if impl.ViaPointer {
		return "*" + impl.Type.Name
	}
	return impl.Type.Name
}
