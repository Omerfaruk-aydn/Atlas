Record something worth carrying into future sessions, or revise what is already recorded.

Two stores, chosen with `scope`:

- `project` — what is true about this codebase and is not already discoverable in it: a build step that is not in the README, a convention the code follows but does not state, the reason behind a decision that looks odd. Do not record what reading a file would tell you.
- `user` — what is true about the person: how they want to be addressed, what they always ask for, a preference they have corrected you on. Never record credentials, tokens, or anything they would not want on disk.

Actions:

- `add` — append one entry, a single line. Adding an entry that is already there changes nothing.
- `replace` — swap `old` for `new`. `old` must appear exactly once; include enough surrounding text to make it unique.
- `remove` — delete the line containing `old`. Same uniqueness rule.
- `set` — rewrite the whole store. Use this to consolidate when a write is refused for being over the limit.

Both stores are bounded, because every entry is prepended to every request from the next session on. A write that would exceed the bound is refused and tells you by how much; consolidate with `set` rather than dropping something at random.

Write sparingly. A store full of the obvious is worse than an empty one: it costs tokens on every request and buries the two or three things that actually matter.
