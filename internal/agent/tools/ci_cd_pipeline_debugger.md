Scan a GitHub Actions workflow file for the handful of mistakes that run green today and turn into a supply-chain risk, a runaway job, or a leaked secret later.

WHEN TO USE THIS TOOL:
- Reviewing a new or changed workflow file before it merges
- Auditing an existing `.github/workflows/*.yml` for the kind of gap that a passing CI run does not surface, because none of these break the build

WHAT IT FLAGS:
- unpinned-action: a `uses:` referencing a branch (`@main`, `@master`, `@latest`, or no `@ref` at all) instead of a version tag or a full commit SHA. A branch ref means the action's code can change without this workflow changing -- exactly the supply-chain risk a pinned dependency is meant to close. Local actions (`./...`) and Docker actions (`docker://...`) are not flagged; they aren't fetched from a mutable ref the same way.
- missing-timeout: a job with no `timeout-minutes`, so a hung step runs until GitHub's own six-hour default cuts it off, burning CI minutes the whole time.
- secret-in-run: a line in a `run:` step that both references `secrets.SOMETHING` and looks like it prints output (`echo`, `print`, `cat`, ...). Printing a secret to the step's own log is one of the most common ways a secret ends up somewhere it shouldn't -- GitHub redacts *known* secret values from logs, but only once they've been registered as used, and a get-around (splitting the string, base64, etc.) defeats that redaction entirely.

PARAMETERS:
- path: path to the workflow file. Required -- there's no single default filename the way a Dockerfile has one.

WHAT THIS DOES NOT DO:
It does not validate the workflow against GitHub's schema, does not run the pipeline, and does not check permissions, matrix strategy, or caching. It reads jobs and steps generically, so an unusual workflow shape (a step defined by a local composite action's own `action.yml`, for instance) is not followed into.
