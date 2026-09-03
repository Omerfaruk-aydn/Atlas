Find everything that would be affected by changing a Go function or method — its transitive callers, ordered by distance.

WHEN TO USE THIS TOOL:
- Before changing a function's signature, behaviour, or error contract, to see the blast radius first
- Before deleting a function, to confirm what depends on it
- When estimating the size of a refactor: how much code has to move with the change
- When a bug is found in one function and you need to know which call paths could have hit it

HOW IT WORKS:
Every .go file is parsed and a reverse call graph is built, then walked breadth-first from the target. Depth 1 is a direct caller, depth 2 calls a depth-1 caller, and so on. No type checking and no build step, so it works on a tree that does not compile.

WHERE IT IS WRONG:
Call sites are matched on the identifier alone: `x.Close()` and `y.Close()` are indistinguishable without a type checker, so every declaration named `Close` is a candidate and every `Close()` call is an edge.

This over-approximates on purpose. For "what breaks if I change this", a caller wrongly listed costs a moment's reading; a caller missed costs a broken build. When the name is ambiguous the output says so and names the other declarations, so you know when to distrust the breadth.

Not seen at all: calls made through reflection, through a function value stored in a variable or struct field, or from a file excluded by build tags.

PARAMETERS:
- symbol: the function or method name. Required. Bare name, no receiver and no package prefix.
- path: directory to scan. Defaults to the working directory.
- max_depth: how many hops to walk. Default 3. Raise it for a full picture on a small package; keep it low on a large one.
- include_tests: also scan _test.go files, which shows which tests cover the change. Default false.

TIPS:
- Confirm a specific caller with lsp_references, which is type-aware where this is not.
- An empty result on a symbol you know is used usually means it is called through an interface or a function value; check type_hierarchy for the interface case.
