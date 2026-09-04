Scan Go source for three syntactic smells that compile cleanly but hide real bugs: swallowed errors, a context.Context that isn't a function's first parameter, and panics used in place of returned errors.

WHEN TO USE THIS TOOL:
- Reviewing a diff or a package before merging, to catch a checked-but-ignored error or a stray panic
- Auditing an unfamiliar codebase for the kind of defect that `go vet` and most linters don't catch
- Before a refactor, to see which functions already break the context-first convention

WHAT IT FLAGS:
- swallowed-error: an `if err != nil { }` with an empty body, or an error-like value discarded with `_ = err`. Both compile and both can hide a real failure.
- context-not-first: a function whose context.Context parameter isn't first, breaking the convention every caller and linter expects.
- panic-in-library: a panic outside package main, a test file, or a `MustXxx` function (where panicking is the documented contract). A library that panics instead of returning an error takes the decision to crash away from its caller.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory.
- include_tests: also scan _test.go files. Off by default, since a test panicking on a bad fixture or discarding a throwaway error is normal.

HOW TO READ THE RESULTS:
This is name- and shape-based, not type-checked -- it never resolves types, so it can flag a `_ =` discard on a value that only looks like an error by name, and it can't see through an error stored under an unusual variable name. Every finding is a candidate for review, not a confirmed bug; read the surrounding code before changing it.
