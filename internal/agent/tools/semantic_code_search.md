Search a Go codebase for the declaration that matches a natural-language description, by scoring keyword overlap against each symbol's name and doc comment.

WHEN TO USE THIS TOOL:
- Getting oriented in an unfamiliar codebase: "where is the retry logic?", "what parses the config file?"
- Finding a helper you remember the purpose of but not the exact name, including an unexported one that grep for an exact string would miss without knowing it first

WHAT IT SEARCHES:
Every top-level func, method, type, const, and var in the tree -- exported or not, since the target of a search like this is just as often an internal helper as a public entry point.

HOW MATCHING WORKS, AND ITS LIMITS:
This is keyword overlap, not an embedding or any real language understanding. Each query word is checked against the symbol's name (split on camelCase and snake_case boundaries -- "generate docstring" matches `GenerateDocstrings`) and its doc comment. A name match counts for more than a doc match, and a doc match counts for more than nothing. That means it finds "which symbol's name or comment shares vocabulary with this query" -- a function that does exactly what you're asking about, but is named and documented with different words, will not surface. Pair this with grep or a directory listing when a search comes back empty; that's a sign the vocabulary doesn't match, not necessarily that the thing doesn't exist.

PARAMETERS:
- path: directory or single file to search. Defaults to the working directory.
- query: the natural-language description to search for. Required.
- limit: maximum number of results. Defaults to 10.
- include_tests: also search _test.go files. Off by default.

WHAT THE RESULTS SHOW:
Each match's kind, signature, file:line, its doc comment's first line if it has one, and which of the query's words actually contributed to its score -- so a result can be checked against the query rather than trusted blindly.
