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

const APISurfaceToolName = "api_surface"

//go:embed api_surface.md
var apiSurfaceDescription string

const (
	// maxAPISymbols bounds one package's listing. A package with a
	// thousand exported symbols is not readable in one answer, and the
	// right move is to narrow rather than to print it all.
	maxAPISymbols = 250
	// maxAPIPackages bounds an unfocused scan.
	maxAPIPackages = 12
)

type APISurfaceParams struct {
	Path             string `json:"path,omitempty" description:"Directory to scan. Defaults to the working directory."`
	Package          string `json:"package,omitempty" description:"Restrict to a package by name or directory suffix."`
	UndocumentedOnly *bool  `json:"undocumented_only,omitempty" description:"List only exported symbols with no doc comment."`
	IncludeTests     *bool  `json:"include_tests,omitempty" description:"Also scan _test.go files. Default false."`
}

type APISurfaceResponseMetadata struct {
	Packages     int `json:"packages"`
	Symbols      int `json:"symbols"`
	Undocumented int `json:"undocumented"`
	Deprecated   int `json:"deprecated"`
	FilesScanned int `json:"files_scanned"`
}

func NewAPISurfaceTool(workingDir string) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		APISurfaceToolName,
		apiSurfaceDescription,
		func(ctx context.Context, params APISurfaceParams, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			root := cmp.Or(params.Path, workingDir)
			if !filepath.IsAbs(root) {
				root = filepath.Join(workingDir, root)
			}

			result, err := codeintel.APISurface(root, params.IncludeTests != nil && *params.IncludeTests)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}

			packages := result.Packages
			if params.Package != "" {
				packages = filterAPIPackages(packages, params.Package)
				if len(packages) == 0 {
					return fantasy.NewTextResponse(fmt.Sprintf(
						"No package matching %q among the %d scanned.", params.Package, len(result.Packages))), nil
				}
			}

			undocumentedOnly := params.UndocumentedOnly != nil && *params.UndocumentedOnly

			symbols, undocumented, deprecated := 0, 0, 0
			for _, pkg := range packages {
				symbols += len(pkg.Symbols)
				undocumented += pkg.Undocumented
				for _, s := range pkg.Symbols {
					if s.Deprecated {
						deprecated++
					}
				}
			}

			return fantasy.WithResponseMetadata(
				fantasy.NewTextResponse(formatAPISurface(packages, result, undocumentedOnly, workingDir)),
				APISurfaceResponseMetadata{
					Packages:     len(packages),
					Symbols:      symbols,
					Undocumented: undocumented,
					Deprecated:   deprecated,
					FilesScanned: result.FilesScanned,
				},
			), nil
		},
	)
}

// filterAPIPackages accepts a package name or a directory suffix, because
// the caller may know either.
func filterAPIPackages(packages []codeintel.APIPackage, want string) []codeintel.APIPackage {
	want = strings.Trim(filepath.ToSlash(want), "/")
	var out []codeintel.APIPackage
	for _, pkg := range packages {
		dir := filepath.ToSlash(pkg.Dir)
		if pkg.Name == want || dir == want || strings.HasSuffix(dir, "/"+want) {
			out = append(out, pkg)
		}
	}
	return out
}

func formatAPISurface(packages []codeintel.APIPackage, result codeintel.APIResult, undocumentedOnly bool, workingDir string) string {
	if result.FilesScanned == 0 {
		return "No Go files found to scan."
	}
	if len(packages) == 0 {
		return fmt.Sprintf("No exported symbols found across %d file(s).", result.FilesScanned)
	}

	var b strings.Builder
	shown := packages
	truncatedPackages := false
	if len(shown) > maxAPIPackages {
		shown = shown[:maxAPIPackages]
		truncatedPackages = true
	}

	for _, pkg := range shown {
		symbols := pkg.Symbols
		if undocumentedOnly {
			filtered := symbols[:0:0]
			for _, s := range symbols {
				if s.Doc == "" {
					filtered = append(filtered, s)
				}
			}
			symbols = filtered
		}
		if len(symbols) == 0 {
			continue
		}

		fmt.Fprintf(&b, "\npackage %s  (%s)\n", pkg.Name, relOrAbs(pkg.Dir, workingDir))
		fmt.Fprintf(&b, "%d exported symbol(s)", len(pkg.Symbols))
		if pkg.Undocumented > 0 {
			fmt.Fprintf(&b, ", %d undocumented", pkg.Undocumented)
		}
		b.WriteString("\n")

		listed := symbols
		if len(listed) > maxAPISymbols {
			listed = listed[:maxAPISymbols]
		}
		for _, s := range listed {
			indent := "  "
			// Fields and methods are printed under their type, which is
			// what makes a type readable as a unit.
			if s.Kind == "field" || s.Kind == "method" {
				indent = "      "
			}
			fmt.Fprintf(&b, "%s%s", indent, s.Signature)
			if s.Deprecated {
				b.WriteString("   [DEPRECATED]")
			}
			b.WriteString("\n")
			if s.Doc != "" {
				fmt.Fprintf(&b, "%s  // %s\n", indent, truncateText(s.Doc, 140))
			}
		}
		if len(listed) < len(symbols) {
			fmt.Fprintf(&b, "  ... and %d more symbol(s) in this package.\n", len(symbols)-len(listed))
		}
	}

	if b.Len() == 0 {
		return "Every exported symbol in the selected package(s) is documented."
	}
	if truncatedPackages {
		fmt.Fprintf(&b, "\n... and %d more package(s). Narrow with `package` or `path`.\n",
			len(packages)-len(shown))
	}
	return strings.TrimLeft(b.String(), "\n")
}
