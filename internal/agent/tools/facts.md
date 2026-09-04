Retain a fact during this session and recall it later by keyword, without waiting for the next session the way project/user memory does.

WHY THIS IS DIFFERENT FROM `memory`:
`memory` is loaded once when a session starts and prepended to every request -- a fact written there is not seen again until the *next* session. This store is the opposite trade: a fact retained now can be recalled a moment later in the *same* conversation, at the cost of spending a tool call to look it up rather than always having it present. Use `memory` for something that should quietly be true every time this project or user comes up again. Use this for something worth not re-discovering later in a long session -- a decision made, a constraint found, a dead end already ruled out -- that doesn't need to survive past it.

ACTIONS:
- retain: record a fact. Params: text (required), tags (optional labels, e.g. ["build", "decision"]).
- recall: search retained facts. Params: query (words to search for; empty returns the most recently retained facts instead), limit (default 10). A tag match counts for more than a text match -- this is keyword overlap, not an embedding, the same honest search semantic_code_search uses for code.
- reflect: summarise the store as a whole -- total count, a tag breakdown, and any facts whose text is a near-exact duplicate of another. This groups and counts; it does not generate prose from the facts, since that would mean calling a model from inside a tool. Use it to decide what's worth consolidating into permanent `memory`, or what's safe to stop tracking.

WHAT THIS DOES NOT DO:
Nothing here is re-read into the conversation automatically -- recall has to be called to see anything back. Nothing here is bounded the way memory is, so it will not fail a write for being too long, but it also does not steer future sessions on its own; a fact worth keeping past this session belongs in `memory` instead.
