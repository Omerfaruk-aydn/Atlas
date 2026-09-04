Build a table of contents across a tree's Markdown files, or search it by keyword.

WHEN TO USE THIS TOOL:
- Getting oriented in an unfamiliar repository's documentation before reading any file in full
- Finding which doc covers a topic ("where is authentication documented?") without grepping file contents by hand
- Checking a docs/ directory's overall structure before adding a new section, to see where it belongs and avoid duplicating an existing one

WHAT IT PRODUCES:
For each Markdown file: its title (the first `# H1`, or the filename when there is none) and its heading outline with line numbers, so a heading can be jumped to directly instead of reading the file top to bottom.

HOW HEADINGS ARE READ:
Only ATX headings (`# Title`, `## Section`, ...) are recognised -- the form the overwhelming majority of real Markdown uses. Setext headings (a line underlined with `===` or `---`) are not detected and will not appear in the outline. A `#` inside a fenced code block is correctly ignored as source, not structure.

PARAMETERS:
- path: directory or single Markdown file to scan. Defaults to the working directory.
- query: restrict the result to files whose title or any heading contains this text, case-insensitively. Omit to list every file.
- max_depth: only include headings at or above this level (1-6). Defaults to 6 (everything).

WHAT IS SKIPPED:
node_modules, vendor, and hidden directories -- a heading in a dependency is not this repository's documentation.
