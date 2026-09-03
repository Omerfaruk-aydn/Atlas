Measure the shape of Go functions — cyclomatic complexity, length, nesting depth, signature arity — to find the code most likely to need attention.

WHEN TO USE THIS TOOL:
- Picking where to start on an unfamiliar codebase: the outliers are usually where the bugs and the domain knowledge both live
- Deciding whether a function needs splitting before you add to it
- Checking whether a refactor actually reduced complexity, by running it before and after
- Finding the functions that most need tests, since branch count is what a test suite has to cover

WHAT IS MEASURED:
- complexity: cyclomatic complexity — 1, plus one per if, for, range, non-default case, branching select clause, && and ||. This matches gocyclo and golangci-lint's cyclop, so the numbers are comparable to those tools.
- lines: the span of the declaration.
- nesting: deepest block nesting. Two functions can share a complexity score while one is flat and the other is a staircase; the staircase is harder to read.
- params / results: signature arity. A long parameter list is a design smell complexity does not capture.
- returns: return statement count.

These measurements are exact. Counting syntax needs no type information, so unlike the other static-analysis tools here there is no approximation.

WHAT THE NUMBERS DO NOT TELL YOU:
A threshold is not a verdict. A 30-branch switch dispatching over an enum is fine and does not want splitting; a 30-branch nest of conditionals is a problem. The tool cannot tell them apart — read the function before acting on its score.

PARAMETERS:
- path: directory or single .go file. Defaults to the working directory.
- top: how many functions to list, most complex first. Default 20.
- min_complexity: only report functions at or above this score. Optional; use it to see how many functions cross a line.
- include_tests: also measure _test.go files. Default false — table-driven tests score high for reasons that are not a problem.

TIPS:
- Combine with impact_analysis on a high-complexity function: complex AND widely called is the highest-risk code in the repository.
- The file summary at the end is a fast way to find the file that has quietly become a dumping ground.
