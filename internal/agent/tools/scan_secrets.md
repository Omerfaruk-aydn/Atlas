Scan a tree for credentials that have been committed by accident — API keys, tokens, private keys, passwords in connection strings.

WHEN TO USE THIS TOOL:
- Before committing or opening a pull request, especially after adding config, fixtures, or example files
- When taking over an unfamiliar repository, to find what is already leaked
- After generating code that touches configuration, since a plausible-looking key is exactly the kind of thing that gets filled in and forgotten
- When a user asks whether their repository is safe to make public

HOW IT WORKS:
Two layers. Named rules match shapes that specific providers issue — AWS, GitHub, GitLab, Slack, Stripe, Google, OpenAI, Anthropic, npm, SendGrid, Twilio, JWTs, private key headers, and passwords embedded in database or HTTP URLs. Those carry high confidence because the shape has essentially no other meaning.

A generic layer catches everything else: a value assigned to a name like `password`, `api_key` or `client_secret` whose content is random enough to be a real key. That layer is where nearly all false positives come from, so placeholder values (`changeme`, `your_key_here`, `${VAR}`, `os.getenv(...)`) and low-entropy strings are dropped, and each finding carries its confidence.

SECRETS ARE NEVER REPRINTED:
Every value is redacted before it leaves the scanner — first four characters, then stars, then the last four. Enough to identify which credential to rotate, never enough to use. Do not go on to read the file and quote the full value: that would put the secret into this transcript and any log of it.

WHAT IS SKIPPED:
node_modules, vendor, .git, build and dist directories, binary and media extensions, lockfiles, files over 1 MB, and lines over 4 KB. A key found in a dependency is not this repository's leak. A line ending in `atlas:allow-secret` or `gitleaks:allow` is honoured, so a documented fixture does not get reported forever.

PARAMETERS:
- path: directory or single file. Defaults to the working directory.
- min_confidence: "high", "medium", or "low". Default reports everything. Use "high" for a fast check with almost no false positives.
- skip_generic: only run the provider-specific rules. Fewer false positives, and misses credentials no rule knows about.

WHAT TO DO WITH A FINDING:
A real leaked credential must be ROTATED, not just deleted. Removing the line does not help — the value is still in git history and, if the repository was ever pushed, is already public. Say this plainly when reporting one.
