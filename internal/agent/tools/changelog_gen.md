Draft a changelog from real git history, grouped and ordered the way a reader wants to read it.

WHEN TO USE THIS TOOL:
- Preparing a release: draft the entry, then edit it rather than writing one from memory
- Reviewing what actually shipped since a tag, before writing release notes
- Checking whether recent commits follow Conventional Commits, and how many do not

HOW GROUPING WORKS:
Commits are read as Conventional Commits (`feat(scope): description`) when they follow that shape. Sections appear in reading order: BREAKING CHANGES first, then Features, Fixes, Performance, Reverts, Documentation, Refactoring, Tests, and Chores (style/build/ci/chore combined, since none of them affect a user). A commit that does not follow the convention is kept under "Other" rather than silently dropped — an omitted commit makes a changelog look complete when it is not, and that is only discovered later by someone looking for a change that is missing.

A breaking change is detected two ways per the spec — a `!` after the type (`feat!:`) or a `BREAKING CHANGE:` footer — and listed in its own leading section while STAYING in its original type section too. A breaking feature is still a feature.

PARAMETERS:
- range: a revision or range, e.g. "v1.2.0..HEAD" for everything since a tag, or "HEAD" for the whole history. Required.
- dir: a directory inside the repository. Defaults to the working directory.

WHAT THIS TOOL DOES NOT DO:
It never writes to CHANGELOG.md or any file — it drafts the text and returns it. Use write/edit yourself if the result should be saved, so the change goes through the normal review the user expects for a file write.

A drafted entry is a starting point. Commit subjects are terse by design; rewrite a description that reads well in isolation but was clearly written only for someone already deep in the diff.
