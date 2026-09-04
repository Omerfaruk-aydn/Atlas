Inspect the currently staged changes (`git diff --cached`) for the kind of mistake that is obvious in hindsight and easy to miss under review.

WHEN TO USE THIS TOOL:
- Right before committing, as a last look at what is actually staged
- After resolving a merge or rebase conflict, to confirm no marker was left behind
- After a debugging session, to catch a stray print statement before it ships

WHAT IT FLAGS:
- merge-conflict-marker: an added line starting with `<<<<<<<`, `=======`, or `>>>>>>>` -- a conflict that was never actually resolved.
- debug-statement: an added line containing `console.log(`, `fmt.Println(`, `debugger`, `dbg!(`, `binding.pry`, or `pdb.set_trace()` -- forms used almost exclusively for throwaway debugging, not real logging.
- large-file: a staged file over the size threshold, the shape of an accidentally staged binary, dump, or dependency directory.

Only added lines are scanned for markers and debug statements -- a line that was already there and is merely being moved is not something this commit introduced.

PARAMETERS:
- dir: a directory inside the repository. Defaults to the working directory.
- max_file_bytes: size threshold for the large-file check. Defaults to 1 MiB.

WHAT THIS DOES NOT DO:
It never touches the index or working tree, and it does not scan for leaked credentials -- use scan_secrets for that. It also does not run tests, linters, or type-checkers; it is a fast, read-only look at the diff itself, not a substitute for the project's actual checks.
