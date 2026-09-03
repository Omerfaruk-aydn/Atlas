Check a Go module's dependencies: which have known vulnerabilities, and which are behind.

WHEN TO USE THIS TOOL:
- Before a release, or before making a repository public
- When adding or upgrading a dependency, to see what it brings with it
- When a security advisory lands and you need to know whether this project is affected
- Periodically on a project that has not been touched in a while — the code did not change, but the advisories did

REACHABLE VS. PRESENT — THIS IS THE POINT:
Vulnerabilities come from govulncheck, which is the only tool that checks whether the vulnerable code is actually reachable from this module rather than merely present in the dependency graph.

The report separates the two, and the difference decides what to do:
- CALLED: the affected function is reachable from this code. This needs action.
- present but not called: the vulnerable version is in the graph, but nothing here reaches the affected code. Worth upgrading, not worth an emergency.

Most "vulnerable dependency" counts from other scanners are the second kind. Reporting them as though they were the first is why those reports stop being read.

WHEN govulncheck IS NOT INSTALLED:
The tool says so explicitly and reports nothing about vulnerabilities. An empty vulnerability list then means NOT CHECKED, not "none found" — never report it as a clean bill of health. Install it with:

go install golang.org/x/vuln/cmd/govulncheck@latest

PARAMETERS:
- skip_updates: skip the outdated-dependency check. That half reaches the network and is much the slower one, so skip it when you only care about vulnerabilities.
- skip_vulns: skip govulncheck and only report what is out of date.
- timeout: how long each command may take. Default 2 minutes.
- dir: a directory inside the module. Defaults to the working directory.

ON UPGRADING:
A newer version existing is not a reason to upgrade. Direct dependencies that are far behind are worth raising; indirect ones usually move when their parent does. Never upgrade a dependency as a side effect of running this — report what you found and let the user decide.
