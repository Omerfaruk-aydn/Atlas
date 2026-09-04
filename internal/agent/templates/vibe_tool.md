Start a persistent background worker that keeps working toward a goal across several of its own turns, independent of this conversation's turn -- and steer it with new guidance while it runs, the way a director gives ongoing notes to someone already at work rather than re-briefing them from scratch each time.

ACTIONS:
- start: launches a worker. Params: goal (required, what to keep working toward), agent_name (optional, a configured subagent to run it on instead of the default agent), max_turns (optional, safety cap on how many of its own turns it may take before stopping on its own even if not done; default 25, hard ceiling 100). Returns the worker's id immediately -- the worker keeps running in the background after this call returns.
- direct: params id (required), note (required). Queues new guidance for the worker to read before its *next* turn -- a turn already in progress does not see it. Use this instead of starting a new worker when you want to redirect one that's already running.
- status: params id (required). Reports the worker's current status (running / done / stopped / failed), how many turns it has taken, and its most recent output.
- list: no params. Lists every worker started from this session, with the same summary status gives for one.
- stop: params id (required). Cancels the worker. Its current turn finishes; no further turn starts.

A worker stops on its own when it replies with exactly "VIBE_DONE" (it is told to, once it judges the goal complete), when it hits max_turns, when stop is called, or if a turn fails outright. Check status periodically rather than assuming a worker either finished or is still going -- max_turns exists specifically so a worker that never says it's done does not run forever.

This is a simpler mechanism than the live turn/queue pipeline behind ordinary prompts: each worker turn is a fresh call carrying only its own progress summary and any queued director notes forward, not this session's full context. Use it for a genuinely long-running, checkable-in-on task (a large refactor to grind through, a batch of files to migrate), not as a substitute for `agent` or `delegate` on a task that finishes in one or a few turns.
