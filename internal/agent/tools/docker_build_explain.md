Parse a Dockerfile into its build stages and flag the handful of instruction-level mistakes that are easy to make and easy to miss reading top to bottom.

WHEN TO USE THIS TOOL:
- Reviewing a new or changed Dockerfile before it merges
- Understanding an unfamiliar multi-stage build -- which stage produces the final image, what each one is FROM
- Auditing a Dockerfile for the kind of issue a human reviewer often only catches after the image ships

WHAT IT PRODUCES:
A per-stage summary (base image, name if given via `AS`) plus findings:
- latest-tag: a FROM with no tag, or an explicit `:latest` -- both resolve to whatever the registry currently serves, so the same Dockerfile can produce a different image tomorrow. A digest pin (`@sha256:...`) and a reference to an earlier build stage are both recognised as fine.
- root-user: no USER instruction anywhere in a stage, so its container runs as root by default.
- add-for-local-copy: an ADD instruction whose source is a local path, not a URL or an archive -- the two things ADD does that COPY doesn't. Using ADD here is usually accidental and worth flagging since it can behave surprisingly (auto-extracting an archive dropped in later).
- secret-in-env: an ENV or ARG assignment whose key looks like a credential (password, token, api key, ...) and whose value isn't an obvious placeholder. Baking a real secret into an image layer is worse than a source-code leak -- it survives even if a later layer or `.dockerignore` tries to remove it, because layer history is part of the image.

PARAMETERS:
- path: path to the Dockerfile. Defaults to "Dockerfile" in the working directory.

WHAT THIS DOES NOT DO:
It does not build the image, run hadolint, or check whether the referenced base images actually exist. It reads the file's instructions only -- correct syntax is assumed, and an instruction it doesn't specifically check (RUN, WORKDIR, EXPOSE, and the rest) is parsed but never flagged.
