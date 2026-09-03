Measure Go test coverage and find the code that no test reaches.

WHEN TO USE THIS TOOL:
- Before writing tests, to find where they are actually missing rather than guessing
- After adding tests, to confirm they reached the code you meant them to reach
- When reviewing a change: is the new logic covered, or only the parts that were already tested?
- When judging how safe a refactor is — untested code is where a refactor breaks things silently

WHAT YOU GET:
The overall percentage, then the least-covered packages and files first, then the specific line ranges that never executed. The line ranges are the actual answer; a percentage on its own tells you there is a problem but not where.

Coverage is measured in STATEMENTS, not lines. A 40-line function with no branches is a single statement block, so a file can be 100% covered while most of its lines never ran individually. Do not translate a percentage into "lines tested".

WHAT COVERAGE DOES NOT MEAN:
Covered means executed at least once, not verified. A test that calls a function and asserts nothing produces full coverage and catches no bugs. High coverage cannot tell you the tests are good; low coverage does tell you code is untested. Read the number in that direction only, and say so when reporting it.

PARAMETERS:
- packages: Go package pattern. Default "./...". Narrow it on a large repository — the run is a real test run and takes real time.
- run: a regexp selecting tests by name, if you want the coverage of a specific test.
- timeout: how long the run may take. Default 5 minutes.
- show_uncovered: list the uncovered line ranges. On by default; turn it off for a bare summary.
- min_percent: report whether coverage meets this threshold.
- dir: a directory inside the module. Defaults to the working directory.

NOTE:
This runs the tests. If they fail, the coverage data is still collected and reported, but the failures are reported first — coverage measured against a failing suite means less than it appears to.
