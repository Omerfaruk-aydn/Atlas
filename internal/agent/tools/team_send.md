Broadcast a short message to every agent in this task's team.

A "team" is the session that started the current task plus every
sub-agent it has spawned via the `agent` tool, however deeply nested.
Membership is automatic: you never register or name a team, you just
send and read.

This exists for sub-agents running in parallel (multiple `agent` tool
calls in the same step) to coordinate with each other while they are
still running, instead of only reporting back through their own final
answer once their whole turn finishes. For example: "I already checked
internal/foo, it's clean" so a sibling doesn't duplicate the work, or
"found the bug in bar.go:42" so others can stop looking.

Keep messages short and factual. This is not a substitute for your
final answer — it is a side channel for avoiding duplicated work and
sharing findings while several agents are active at once.
