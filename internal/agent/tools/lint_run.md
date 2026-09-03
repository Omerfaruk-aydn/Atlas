Run the project's Go linter and get back the findings, grouped by file.

WHEN TO USE THIS TOOL:
- After writing or changing Go code, before saying it is done
- Before committing, to catch what review would otherwise catch
- When a CI lint step failed and you need the same findings locally
- When taking on an unfamiliar codebase, to see what standards it holds itself to

WHICH LINTER RUNS:
golangci-lint if it is installed, honouring the repository's own .golangci.yml so the findings match what CI will say. If it is not installed, `go vet` runs instead — vet ships with the toolchain, so there is always something useful to report. The output always names which one ran, and says explicitly when it fell back, because vet finds considerably less: it catches printf mismatches and lock copies, not unchecked errors, unused values, or style.

Treat a clean `go vet` result as "nothing obviously broken", not as "this code passes review".

PARAMETERS:
- packages: Go package pattern. Default "./...". Narrow it on a large repository; a full lint run is slow.
- linters: restrict to named checks, e.g. ["errcheck", "govet"]. Ignored under the vet fallback.
- timeout: how long the run may take. Default 2 minutes.
- force_vet: skip golangci-lint even if installed. Useful for a fast sanity check.
- dir: a directory inside the module. Defaults to the working directory.

READING THE RESULT:
Findings are grouped by file and ordered by position, so fixing one file means one pass through it. A per-linter tally follows: many findings from one linter usually means a single systemic pattern with one fix, while findings spread across many linters usually means several unrelated problems.

Not every finding needs fixing. A linter encodes opinions, and some of them will be wrong for a given piece of code. Read the message before changing anything, and say so if you decide a finding should be ignored rather than silently working around it.
