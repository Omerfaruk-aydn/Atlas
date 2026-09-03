Find Go declarations that nothing in the scanned tree references — candidates for deletion during a cleanup pass.

WHEN TO USE THIS TOOL:
- Before a refactor, to see what can be removed first and shrink the surface you have to change
- When auditing a package for accumulated cruft
- After deleting a feature, to find the helpers it left behind

HOW IT WORKS:
The scan parses every .go file under the given path and reports each top-level declaration (func, method, type, const, var) whose name is never mentioned anywhere else in that tree.

WHAT IT WILL NOT CATCH, AND WHERE IT IS WRONG:
This is a syntactic heuristic, not a proof. Treat every result as a candidate to review, never as safe to delete on sight:
- A method that exists only to satisfy an interface is never named at the call site, so it will be reported even though removing it breaks the build.
- Symbols reached by reflection, struct tags, code generation, or a build-tagged file that was skipped will be reported.
- An exported symbol used by a downstream module will be reported, because only the given tree is scanned.
- Two different declarations sharing one name are counted together, so a use of either keeps both alive.

PARAMETERS:
- path: directory or single .go file to scan. Defaults to the working directory.
- include_tests: also scan _test.go files. Default false, so a symbol used only by tests IS reported — usually what a cleanup pass wants. Set true to see only what is dead including tests.
- include_unexported: also report unexported symbols. Default true.

TIPS:
- Verify a candidate with lsp_references before removing it — that check is type-aware and this one is not.
- Start with one package rather than the whole repository; the output is far easier to act on.
