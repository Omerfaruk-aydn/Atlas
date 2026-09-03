Find out when each line of a file was last changed, by whom, and in which commit.

WHEN TO USE THIS TOOL:
- "Why is this line here?" — the commit message that introduced it usually says
- Before changing code you do not understand: the commit gives you the intent, and the author gives you who to ask
- Dating code: whether a workaround is from last week or five years ago changes what to do about it
- Finding the commit that introduced a bug, once you know the line

WHAT YOU GET:
Consecutive lines from the same commit are grouped into blocks, because that is how blame is actually read — a block of lines arrived together for one reason. Each block gives the commit, author, date and subject.

An author summary follows: who owns how many lines, and how recently they touched it. That is the answer to "who should review this" or "who would know".

PARAMETERS:
- path: the file to blame. Required.
- start_line / end_line: 1-based, inclusive. Blaming a whole large file is rarely what you want — narrow to the region you are actually looking at.
- rev: blame the file as of this revision instead of the working tree.
- ignore_whitespace: stop a reformatting commit from claiming authorship of every line it merely reindented. Turn this on whenever the answer looks suspiciously like one commit touched everything.
- show_lines: include the source text beside each block. Off by default, since you usually already have the file open.
- dir: a directory inside the repository. Defaults to the working directory.

WHAT BLAME DOES NOT TELL YOU:
The last commit to touch a line is not necessarily the one that introduced the logic. A rename, a reformat, a move between files, or a mechanical refactor all reset the attribution. If a block's commit message does not explain the line, follow it back with git_log on the file.
