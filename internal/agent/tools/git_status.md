Read the git working tree state — what is staged, what is modified, what is untracked, and how the branch stands against its upstream.

WHEN TO USE THIS TOOL:
- Before committing, to see exactly what would go in
- Before starting work, to find out whether the tree is already dirty
- After an edit, to confirm which files actually changed
- When a merge or rebase is in progress and you need the conflicted paths
- Whenever you were about to run `git status` through the shell

PREFER THIS OVER RUNNING git status IN THE SHELL:
It reads `--porcelain=v2`, which is the only status format git documents as stable for machines, and returns a compact structured summary instead of paragraphs of hint text. It cannot modify anything, so it needs no approval and cannot go wrong.

WHAT YOU GET:
- Branch name, or the short commit when HEAD is detached
- Upstream branch and how far ahead/behind
- Each changed path with its staged and unstaged state separately — a file can be modified in both, and committing then captures only half of it
- Renames with the path the content came from
- Conflicted paths called out on their own, because nothing else can proceed until they are resolved

PARAMETERS:
- path: a directory inside the repository. Defaults to the working directory.

NOTES:
- "not a git repository" comes back as a plain answer, not an error.
- Untracked files are listed individually, not collapsed into their directory.
