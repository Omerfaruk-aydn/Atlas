Read the tail of a log file, optionally filtered by substring or log level, without loading the whole file into memory.

WHEN TO USE THIS TOOL:
- Checking a log file's recent activity without reading it in full, especially one too large to read comfortably
- Narrowing to just the errors, or just the lines mentioning a specific request ID or component, in one call instead of reading everything and scanning by eye

PARAMETERS:
- path: path to the log file. Required.
- lines: how many trailing (matching, if filtered) lines to return. Defaults to 100.
- grep: restrict to lines containing this substring, case-insensitively.
- level: restrict to lines that look like they carry this log level -- error, warn, info, debug. Matches as a whole word, case-insensitively, and recognises the common shapes a level shows up in: `ERROR`, `[error]`, `level=error`, `"level":"error"`.

grep and level can be combined; a line must satisfy both to be included.

HOW TO READ THE RESULT:
`total_lines` in the response metadata is the file's line count before any filtering -- if a filter matches nothing, checking this tells you whether the file is just short or the pattern genuinely doesn't occur. `truncated: true` means more matching lines existed than `lines` could hold; the ones actually shown are still the most recent, in order.

WHAT THIS DOES NOT DO:
It reads a static snapshot of the file once -- it does not follow the file for new lines the way `tail -f` does, and it does not parse structured log formats (JSON, logfmt) into fields; `level` and `grep` both match against the raw line text.
