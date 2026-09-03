Search and read commit history — by path, author, message, date range, or branch.

WHEN TO USE THIS TOOL:
- "Why is this code like this?" — read the commits that produced it
- Before changing a file, to see who has been in it recently and what they were doing
- Finding when a bug was introduced, or when a feature landed
- Writing a changelog or release notes from what actually shipped
- Understanding an unfamiliar area: the commit messages on a directory are usually the fastest orientation available

PREFER THIS OVER RUNNING git log IN THE SHELL:
It parses git's output with separators that cannot occur in commit metadata, so multi-line commit bodies survive intact instead of being truncated at the first newline. It is read-only, so it needs no approval.

PARAMETERS:
- path: restrict to commits touching this file or directory. This is usually the most useful filter.
- limit: how many commits. Default 20.
- author: name or email substring.
- grep: commit message substring. Matched literally, so "fix(auth)" means that text and not a regular expression.
- since / until: anything git accepts, including "2 weeks ago" or "2026-01-01".
- ref: a branch, tag, or revision range such as "main..feature". Empty means the current HEAD.
- with_stats: include the files each commit touched and its line counts. Costs an extra git call per commit, so leave it off when scanning many.
- no_merges: drop merge commits, which usually carry no content of their own and can dominate a busy history.
- dir: a directory inside the repository. Defaults to the working directory.

TIPS:
- `path` plus `with_stats` is the best answer to "what has been happening in this module".
- A revision range in `ref` ("main..HEAD") is how to see exactly what a branch adds.
- Commit messages are written by people and may be wrong, stale, or aspirational. Read them as evidence about intent, not as a description of what the code currently does.
