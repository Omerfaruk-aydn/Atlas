Propose how to split a pile of uncommitted changes into several smaller commits, ordered so that a package another changed package depends on is committed first.

WHEN TO USE THIS TOOL:
- Before committing a working tree with changes across several unrelated areas, to see a sensible split instead of one large commit
- After a multi-part task (a refactor plus the feature that used it, say) to separate them into commits that each stand on their own

WHAT IT PRODUCES:
One group per directory that changed. For a Go package, dependency order is computed from actual import declarations: if a changed package imports another changed package, the imported one's group comes first, so committing group by group in the order shown never leaves an in-between state that references code that doesn't exist yet. Non-Go directories (docs, config, CI) are not import-ordered -- there's no such relationship to compute -- and are listed after every Go group, sorted alphabetically.

Each group lists its files and total insertions/deletions. Reads every kind of working-tree change: staged, unstaged, and untracked new files alike. A conflicted (unmerged) path is left out of every group -- it can't be committed until it's resolved.

WHAT THIS DOES NOT DO:
It never runs `git add` or `git commit` -- nothing here changes the repository. Turning a group into a real commit (`git add <files>` then commit) and writing that commit's message is a separate, deliberate step; pair this with git_conventional_commit, run once per group after staging just its files, to get a type/scope suggestion for it.

WHEN GROUPS CAN'T BE FULLY ORDERED:
Two changed packages that import each other (a cycle) cannot be placed strictly one before the other -- each would reference the other's new code. Such packages are merged into one group instead of two, and reported under `cycles` in the response, so the reason they're together is visible rather than silently ordered by guesswork.

PARAMETERS:
- dir: a directory inside the repository. Defaults to the working directory.
