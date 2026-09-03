See what changed — in the working tree, in the index, or between any two revisions.

WHEN TO USE THIS TOOL:
- Before committing, to review exactly what is about to go in
- After making edits, to confirm you changed what you meant to and nothing else
- Reviewing a branch: `ref: "main..HEAD"` shows exactly what it adds
- Understanding what a commit did: `ref: "<hash>~1..<hash>"`
- Checking whether a file you thought you edited actually changed

WHAT IT COMPARES (this is the part to get right):
- default: working tree vs. the index — your unstaged edits
- staged: true: the index vs. HEAD — exactly what a commit would capture
- ref: "main": working tree vs. that revision
- ref: "main..HEAD": one revision against another, touching neither

Staged and unstaged are different questions. Asking the wrong one is how a commit ends up containing something other than what was reviewed.

SUMMARY FIRST, PATCH ON REQUEST:
By default you get the file list with line counts, which is exact and cheap. Set with_patch to get the actual diff text. Leave it off when you only need to know *what* changed; turn it on when you need to see *how*. A large refactor's full patch can be enormous, so with_patch is bounded and says when it dropped text — the summary is always complete.

PARAMETERS:
- staged: compare the index against HEAD.
- ref: a revision or range to compare against.
- path: narrow to one file or directory.
- with_patch: include the unified diff text.
- context_lines: unchanged lines around each hunk. Default 3. Lower it to fit more changes; raise it when the surrounding code matters.
- dir: a directory inside the repository. Defaults to the working directory.

NOTES:
- Renames are expanded into both real paths, so you can read either side.
- Binary files are listed but not diffed.
