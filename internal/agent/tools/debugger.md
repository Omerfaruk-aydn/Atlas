Runs a Go program under Delve, a real debugger: set breakpoints, step through execution, and inspect live variables. Scoped to Go only. Useful when a bug depends on runtime state a test or a log statement can't easily surface -- what a variable actually holds three calls deep, whether a branch is really taken, what the call stack looks like at the moment something goes wrong.

One session stays open across calls within a chat session, so a multi-step flow (set a breakpoint, continue, inspect, step) all happens on the same run. `start` again replaces whatever was already running; `stop` ends it explicitly.

Actions (set `action` to one of these):
- `start` — build and launch `program` (a package directory, most commonly, or a single .go file) with `args`, paused before any code runs. The response reports the initial stop.
- `breakpoint` — set breakpoints at `lines` in `file`, replacing any already set in that file. An empty `lines` clears them. The response reports whether each one verified (Delve can fail to verify a breakpoint on an unreachable line, e.g. inside a comment or a function that got inlined away).
- `continue` — resume until the next breakpoint, a step target, or the program exits.
- `next` — step over the current line, without entering calls it makes.
- `step_in` — step into the call on the current line.
- `step_out` — run until the current function returns.
- `stack` — the current call stack.
- `variables` — the current frame's local variables and arguments, grouped by scope. Pass `frame_index` (0 = innermost, the default) for an outer frame. A structured value (a struct, slice, map) shows `[ref=N]`; pass that as `variables_reference` to expand its fields instead of listing scopes.
- `eval` — evaluate `expression` in the paused program's current context (same `frame_index` semantics as variables).
- `output` — whatever the program has printed to stdout/stderr since the last time this was called.
- `stop` — end the debug session.

Guidance:
- `continue`/`next`/`step_in`/`step_out` all report where execution stopped next (or the exit code, if the program ran to completion) -- there's no separate "wait" step.
- A `start` failure is almost always a build error; the response includes the compiler's own output, not just "build failed".
- Requires Delve (`dlv`) installed on the host; `atlas doctor` reports whether one is available when this tool is turned on.
