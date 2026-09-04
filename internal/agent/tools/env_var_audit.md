List every environment variable a Go codebase reads or writes via `os.Getenv`, `os.LookupEnv`, and `os.Setenv`, and cross-check the ones read against an example env file.

WHEN TO USE THIS TOOL:
- Onboarding to an unfamiliar service, to see every environment variable it depends on in one list instead of grepping for `os.Getenv` by hand
- Before a deploy or a new environment setup, to catch a variable the code reads that `.env.example` never mentions
- Reviewing a PR that adds configuration, to confirm the new variable was also added to the example file

WHAT IT FINDS:
Every `os.Getenv("X")` and `os.LookupEnv("X")` call (both reported as "read"), and every `os.Setenv("X", ...)` call ("write"). A call whose key isn't a string literal -- built from a variable, a format string, concatenation -- is still counted but reported as `(dynamic)`, since its actual name can't be read off the syntax.

DOCUMENTATION CROSS-CHECK:
If a `.env.example`, `.env.sample`, or `.env.dist` file exists directly under the scanned path (the first one found, in that order), every literal variable name that is read gets checked against it. Names present in the code but missing from that file are reported as undocumented. When no such file exists at all, this check is skipped entirely -- there being no undocumented variables is not the same as there being nothing to compare against, and the response says which case applies.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory.
- include_tests: also scan test files. Off by default.

WHAT THIS DOES NOT DO:
It does not know whether an undocumented variable is actually required or merely optional with a sensible default handled elsewhere in the code, and it does not validate the example file's own values -- only that a name mentioned in code also appears as a key in it.
