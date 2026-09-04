Look inside a file whose format a plain read can't meaningfully show: a .zip/.tar/.tar.gz archive, a SQLite database, or a Jupyter .ipynb notebook. The format is detected automatically from the path (by extension for archives and notebooks, by file header for SQLite) -- there is no need to say which kind of file it is.

ACTIONS:
- list (default): archive -> every entry's name, size, and whether it's a directory. SQLite -> every table and its columns. Notebook -> every cell's index, type, and first line, so you can see the shape before reading one in full.
- read: archive -> one entry's content as text (params: entry). Notebook -> one cell's full source and output summary (params: cell, 0-based index). Not meaningful for SQLite -- use query instead.
- query: SQLite only. Runs a read-only SQL query (params: query, limit -- default 50 rows). The connection itself is opened read-only at the SQLite level, so a write statement fails outright rather than being screened by string matching.

WHAT THIS DELIBERATELY DOES NOT COVER:
PDF (no dependable Go standard-library parser; adding a third-party one for this alone hasn't been worth it) and anything reached over a network or by credential (SSH, remote databases) -- both excluded on purpose rather than missed. Read a PDF's text with whatever extraction the project already has, if any; for a remote resource, use the appropriate network tool instead.

An archive entry and a notebook cell's output are both capped at 1 MiB of text -- past that, the response says so and returns only the first 1 MiB. That is a display limit for a terminal-facing tool, not a way to process large binary payloads.
