Read messages other agents in this task's team have sent with
team_send.

A "team" is the session that started the current task plus every
sub-agent it has spawned, however deeply nested — see team_send for
what this is for.

Pass `since` (the seq_after value from a previous team_send/team_read
call) to skip messages you have already seen. With no arguments this
returns everything sent so far.

By default this returns immediately, even if nothing is there yet
("No new team messages."). Set `wait_seconds` (up to 60) to block
instead, useful right after telling a sibling agent you're waiting on
something from it. Do not set it to a large value speculatively — a
long wait still counts against this turn.
