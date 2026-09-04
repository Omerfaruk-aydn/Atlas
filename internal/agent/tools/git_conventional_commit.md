Suggest a Conventional Commits type and scope for the currently staged changes, inferred from which files changed -- not from what the change is for.

WHEN TO USE THIS TOOL:
- Right before writing a commit message, to get the "type(scope):" prefix right without guessing at the project's own conventions
- When staged changes span several files and it isn't obvious at a glance whether this reads as a feature, a fix, or a chore

WHAT IT PRODUCES:
- type: one of feat, fix, docs, test, ci, build, chore -- see confidence below
- scope: the deepest directory shared by every changed file, e.g. "gitx" for changes confined to internal/gitx. Empty when the changes don't share one, or when the shared ancestor is only one segment deep (too generic, like "internal" or "src", to say anything).
- confidence: high, medium, or low
- a breakdown of changed files by category (source, test, docs, ci, dependency, config)

HOW THE TYPE IS INFERRED, AND WHY CONFIDENCE VARIES:
When every changed file falls into one obvious bucket -- all tests, all docs, all CI config, all dependency manifests -- the type is high-confidence: test, docs, ci, or build respectively. Once source files are involved, this tool has no way to know intent from the diff alone. It falls back to a weaker signal -- new files present suggests feat (medium confidence), only deletions suggests chore (medium), and plain modification of existing source is reported as fix at LOW confidence with an explicit note that it could just as easily be a feat or a refactor. Treat anything below "high" as a starting point, not the answer -- read the diff yourself and override the type if it's wrong.

PARAMETERS:
- dir: a directory inside the repository. Defaults to the working directory.

WHAT THIS DOES NOT DO:
It does not write the commit message's description -- that requires knowing why the change was made, which this tool cannot read off a diff. Compose that yourself; use pre_commit_guard first to make sure nothing embarrassing is staged.
