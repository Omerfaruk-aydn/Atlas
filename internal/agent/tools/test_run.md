Run Go tests and get back a structured result — what failed, why, and how long it took.

WHEN TO USE THIS TOOL:
- After making a change, to confirm it did not break anything. This is the main one.
- Before claiming a fix works — running the tests is the difference between "should work" and "works"
- Narrowing down a failure with the `run` filter
- Checking whether a failure you are seeing is pre-existing, by running the same tests before and after your change

PREFER THIS OVER RUNNING go test IN THE SHELL:
It drives `go test -json`, the toolchain's only stable machine interface, so subtests, parallel interleaving and build failures are read correctly rather than scraped out of human-formatted text. Output from passing tests is dropped and failures are sorted to the top, so the answer stays readable on a suite with thousands of tests.

WHAT IT GETS RIGHT THAT MATTERS:
A package that does not compile runs zero tests. Reporting that as "0 failures" is the most dangerous wrong answer available, so a build failure is reported as a build failure, with the compiler's message, and the run is never called OK.

PARAMETERS:
- packages: a Go package pattern. Default "./...". Narrow it ("./internal/foo/...") on a large repository — it is much faster.
- run: a regexp selecting tests by name, exactly like `go test -run`. "TestFoo" or "TestFoo/subtest".
- timeout: how long the run may take. Default 5 minutes.
- count: pass 1 to bypass the test cache and force a real re-run.
- verbose: keep output from passing tests too. Off by default.
- race: enable the race detector. Requires cgo; if the toolchain cannot, the run says so.
- dir: a directory inside the module. Defaults to the working directory.

TIPS:
- Start narrow. `packages: "./internal/thing/..."` after touching that package answers the real question in seconds.
- A cached PASS is still a PASS, but if you need to be certain the code actually ran, set count: 1.
- If a test fails, read the output it carries here before opening the file — it usually names the assertion and the line.
