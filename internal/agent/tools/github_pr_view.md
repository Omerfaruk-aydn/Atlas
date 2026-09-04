Read a GitHub pull request's metadata and diff through the gh CLI -- title, body, author, branches, and the full unified diff -- without cloning it or leaving this conversation.

WHEN TO USE THIS TOOL:
- Reviewing a pull request someone linked, without switching to a browser
- Understanding what a PR changes before checking it out, to decide whether checking it out is even necessary
- Pulling a PR's description into context to answer a question about why a change was made

PARAMETERS:
- ref: which PR to fetch. Accepts a bare number ("42" or "#42", resolved against the repository in `dir`'s git remote the same way `gh pr view` does on its own), "owner/repo#42", or a full `https://github.com/owner/repo/pull/42` URL. Required.
- dir: a directory inside the repository, used only to resolve a bare number against the right remote. Defaults to the working directory. Irrelevant when ref already names an owner/repo.

REQUIREMENTS:
This shells out to the gh CLI -- the same tool this project's own PR workflow relies on -- rather than a separate GitHub API client, so it needs `gh` installed and authenticated (`gh auth login`) the same way any other gh command would. A missing or unauthenticated gh is reported plainly rather than as a stack trace.

WHAT THIS DOES NOT DO:
It is read-only: nothing here can create, comment on, approve, merge, or close a pull request. If the diff itself fails to fetch (a PR too large for gh to render inline, for instance) the metadata is still returned -- an empty diff in the response means exactly that, not that the PR has no changes.
