List the repository's branches with their age, divergence, and whether they have already been merged.

WHEN TO USE THIS TOOL:
- Working out where you are: which branch, how far from its upstream, what else is in flight
- Finding the branch that holds some work you remember but cannot name
- Cleaning up: which branches are merged and safe to delete, and which are not
- Before starting work, to see whether a branch for it already exists

MERGED IS THE IMPORTANT COLUMN:
A branch already merged into the base carries nothing that would be lost by deleting it. An unmerged branch does. The listing marks each one, and pass `merged_base` (or let it detect main/master) to get that answer.

Note what "merged" means here: reachable from the base branch. A branch whose commits were squash-merged or rebased onto the base will NOT show as merged, because its original commits no longer exist there — the work is safe, but this tool cannot tell that from genuinely unmerged work. Never present an unmerged branch as safe to delete on this evidence alone.

STALE BRANCHES:
Each branch shows its last commit's age and author. A branch untouched for months is usually either abandoned or forgotten; the author is who to ask.

PARAMETERS:
- include_remote: also list remote-tracking branches. Off by default, since they usually double the list without adding information.
- merged_base: the branch to check merges against. Defaults to the repository's detected default branch (from origin/HEAD, else main/master/trunk/develop).
- stale_days: mark branches whose last commit is older than this many days. Default 60.
- dir: a directory inside the repository. Defaults to the working directory.

THIS TOOL ONLY READS.
It never deletes, creates, or switches branches. If a branch should be deleted, say which and why, and let the user run it.
