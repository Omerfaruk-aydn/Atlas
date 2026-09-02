Search what was said in earlier sessions in this workspace.

The index covers prose only: the text of user and assistant messages, and the command lines of shell calls. Tool output — file contents, diffs, listings — is not indexed, so this finds the conversation, not the code. Use `grep` for the code.

Give `query` the words you would say out loud, not a query language: every word is matched literally and all of them must appear in the same message. A trailing `*` matches by prefix, so `migrat*` finds migration and migrating.

Pass `session_id` to search inside one session; leave it out to search the whole workspace.

Use this when the answer is likely something already decided rather than something written down: why an approach was abandoned, what the user asked for last time, what a previous run of a long task got through. Do not use it as a substitute for reading the current code.
