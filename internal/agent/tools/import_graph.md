Map how a Go module's packages depend on each other, and find import cycles.

WHEN TO USE THIS TOOL:
- Before moving code between packages, to see what the move would drag along
- When a build fails with "import cycle not allowed" and you need the whole loop, not just the two packages the compiler named
- When judging where a new package should live, or whether a package has grown too many dependencies
- When looking for the leaf packages that are safe to change in isolation

MODES (the `focus` parameter):
- omitted: a summary — package count, every cycle, and the most-depended-on packages
- a package import path or directory (e.g. "internal/agent" or "example.com/m/internal/agent"): that package's direct imports and, separately, every package that imports it

HOW IT WORKS:
Each directory of .go files is one package, which is how Go itself defines it. Only the import block of each file is parsed, so a whole-repository scan is cheap. Import paths are resolved through the module path in go.mod; without a go.mod, packages are identified by their path relative to the scan root.

Cycles are found with a depth-first search, and each distinct loop is reported once no matter how many entry points reach it.

WHERE IT IS WRONG:
Only static import declarations are read. An import that appears solely in a file excluded by build tags, or a dependency expressed through a plugin or reflection, is invisible — so the graph can be missing an edge, and a cycle it does not report may still exist. What it does report is real.

PARAMETERS:
- path: directory to scan. Defaults to the working directory.
- focus: package import path or directory to detail. Optional.
- include_tests: also scan _test.go files. Default false — test-only imports legally form cycles that the compiler permits, so including them produces cycle reports that are not build errors.

TIPS:
- Read the cycle output as a loop: each package imports the next, and the last imports the first. Breaking any one edge breaks the cycle.
- "imported by" is the list to check before changing a package's exported API.
