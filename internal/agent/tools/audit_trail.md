Walk a file's history through the lens of one function, using git's own line-history search -- who changed it, when, and exactly what changed in it, commit by commit.

WHEN TO USE THIS TOOL:
- Understanding why a function looks the way it does -- what it used to do, and what each change was for
- Tracking down when a specific bug was introduced into a function, without reading the whole file's history
- Reviewing a function before changing it, to see how often and how recently it has moved

WHAT IT PRODUCES:
Every commit that touched the function, most recent first: hash, author, date, commit subject, and a diff scoped to just that function -- not the whole commit's patch, which is often about something else entirely in a large commit.

PARAMETERS:
- dir: a directory inside the repository. Defaults to the working directory.
- path: the file the function is in. Required.
- symbol: the function name. Required.
- limit: how many commits to return. Omit for git's own default (all of them).

HOW THIS WORKS, AND ITS LIMITS:
This uses `git log -L :function:file`, the same mechanism `git log -L` uses at the command line -- it finds the function by name using git's built-in language-aware heuristics, then follows the lines it occupies backward through history, re-locating the function in each earlier revision. It can lose track across a rename or a heavy restructuring of the surrounding file, the same way `git blame` can. A method, not just a plain function, is supported as long as git's own heuristic recognises it -- if the search reports no match, the tool says so rather than returning an empty, misleadingly-clean history.
