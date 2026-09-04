Find exported Go declarations that have no doc comment and propose a ready-to-paste stub for each, shaped from the declaration's own signature.

WHEN TO USE THIS TOOL:
- Before a release or a PR, to see which exported names in a package still lack documentation
- When asked to "document this file/package", as a starting scaffold to fill in rather than writing every comment from scratch
- Auditing an unfamiliar package for gaps in its public surface's documentation

WHAT IT PRODUCES:
For each undocumented exported func, method, or type, a stub following Go's doc-comment convention (the comment starts with the declared name):
- A one-line summary placeholder
- A "Parameters:" list naming each parameter and its type, when there is more to describe than a single context.Context
- A "Returns" note, calling out separately when the last result is an error

The prose itself is left as TODO -- this tool reads syntax, not intent, so it can shape the comment but cannot know what the code is for. Fill in the placeholders before committing; do not paste the stub as-is.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory.
- symbol: restrict suggestions to one exported name. Omit to scan everything undocumented under path.
- include_tests: also scan _test.go files. Off by default, since exported names in test files are rarely part of a public surface.

NOTE:
This only flags *missing* comments; it does not judge whether an existing comment is any good. For a broader view of what a package exports, pair this with api_surface.
