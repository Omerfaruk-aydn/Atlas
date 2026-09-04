Draft a pull-request description from the actual commits and diff of a branch, not from memory of what you meant to do.

WHEN TO USE THIS TOOL:
- Opening a PR: draft the description from what really changed, then edit it
- Before opening one, to sanity-check that the branch matches what you intended — a diff touching more directories than expected is worth noticing before a reviewer does
- Writing a summary of work spread across many commits

WHAT IT GATHERS:
Every non-merge commit in base..head, read as Conventional Commits where they follow that shape; the full diff's file and line-count summary; which top-level directories changed and how much (a change concentrated in one package reads very differently from one that touches ten); whether any test file changed; and any issue references (#123, PROJ-42) found in the commit messages.

WHAT IT DRAFTS:
A Summary line, a Changes section grouped like a changelog (features, fixes, etc.), the directories touched, whether tests were included, and any ticket references found — so they can be linked rather than retyped.

PARAMETERS:
- base: the branch or ref the PR would merge into, e.g. "main". Required.
- head: the branch being described. Defaults to the current branch.
- dir: a directory inside the repository. Defaults to the working directory.

THIS TOOL DOES NOT TALK TO GITHUB (OR ANY FORGE).
It reads only local git history and produces text. It never opens, updates, or comments on a pull request — creating or editing one is the kind of action that needs the user's explicit go-ahead, and belongs to `gh` or an MCP tool, not this one.

Read the draft before using it. "No tests changed" is worth a second look if the diff touches logic; a summary line that just restates the top commit message is worth improving.
