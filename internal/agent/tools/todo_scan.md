Find the TODO, FIXME, HACK, XXX, BUG, OPTIMIZE and DEPRECATED markers left in a codebase.

WHEN TO USE THIS TOOL:
- Taking over an unfamiliar codebase: the markers are where the previous authors knew something was wrong
- Before a release, to check nothing was left half-finished
- When asked "what still needs doing here" — this is the answer the code itself gives
- Before rewriting an area, to find the warnings already written about it

WHAT IT GETS RIGHT:
Markers are matched only where a comment actually opens them, so "renderTodos", "todoCount", and prose about a todo list do not match. A scanner without that anchoring returns unusable noise on any project that manages a todo list, which is most of them.

Every common comment style is recognised — `//`, `#`, `--`, `/* */`, `<!-- -->`, and continuation lines in block comments — so a polyglot repository is covered rather than just its Go or JavaScript half.

WHAT YOU GET PER MARKER:
- kind, normalised (OPTIMISE and OPTIMIZE are one marker, not two)
- owner, from `TODO(alice):` — an owned marker is one somebody can be asked about
- ticket reference, from `#1234` or `PROJ-42` in the text — a marker with a ticket is tracked somewhere else and usually not yours to act on
- file, line, and the text itself

PARAMETERS:
- path: directory or single file. Defaults to the working directory.
- kinds: restrict to specific markers, e.g. ["FIXME","BUG"]. FIXME and BUG are usually the ones worth reading first — TODO is often aspirational, FIXME is usually somebody recording a known defect.
- extensions: restrict to file types, e.g. ["go","ts"].
- include_tests: also scan test files. Off by default.
- max_results: cap the findings. Default 500.

READING THE RESULT:
A count is not a task list. Many TODOs are years old, already done, or notes to nobody. Look at what they say before treating any of them as work: the useful signal is usually a FIXME or BUG with an owner or a ticket, not the raw total.
