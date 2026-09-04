Read a file with merge-conflict markers and summarize each conflicted region: what each side changed, and how many lines are involved.

WHEN TO USE THIS TOOL:
- After a merge or rebase leaves a file with `<<<<<<<` / `=======` / `>>>>>>>` markers, to get an overview before diving into each block by hand
- On a file with several separate conflicts, to see how many there are and roughly how large each one is before deciding an order to resolve them in

WHAT IT PRODUCES:
For each conflicted region: the line range, the label on each side (usually a branch name or `HEAD`), a line count for each side's version, and a short preview of each. A diff3-style conflict (one that also carries a `|||||||` common-ancestor section) is recognised and its base content shown too -- most repositories use git's default merge style and never have this section.

PARAMETERS:
- path: path to the file to read. Required.

WHAT THIS DOES NOT DO:
It does not resolve anything -- no markers are removed, no side is chosen, and the file is never modified. It only reads and summarizes; picking (or hand-merging) the correct content for each block is still a decision that needs the actual code's context, which a marker's presence alone doesn't carry. A file with no markers reports zero conflicts, which usually means it was already resolved or never conflicted in the first place.
