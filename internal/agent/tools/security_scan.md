Scan Go source for five syntactic security smells that compile cleanly but create real risk: hardcoded credentials, broken cryptographic primitives, disabled TLS verification, and SQL or shell commands built with string formatting instead of parameterization.

WHEN TO USE THIS TOOL:
- Reviewing a diff or a package before merging, to catch a secret someone typed in as a literal or a query built with fmt.Sprintf
- Auditing an unfamiliar codebase for security smells that go vet and most linters don't catch
- Before deploying, as a quick sweep for InsecureSkipVerify or a leftover crypto/md5 import

WHAT IT FLAGS:
- hardcoded-credential: a variable or constant named like a password/secret/token/API key, assigned a string literal that isn't an obvious placeholder ("", "changeme", "TODO", something shorter than 8 characters).
- weak-crypto: an import of crypto/md5, crypto/sha1, crypto/des, or crypto/rc4 -- all broken or deprecated for security use.
- insecure-tls: a struct literal setting `InsecureSkipVerify: true`, which disables TLS certificate verification entirely.
- sql-injection-risk: a call to Query/QueryContext/QueryRow/QueryRowContext/Exec/ExecContext whose first argument is built with fmt.Sprintf instead of a parameterized placeholder.
- command-injection-risk: an exec.Command call with an argument built via fmt.Sprintf instead of passed as a separate, literal argument.

PARAMETERS:
- path: directory or single file to scan. Defaults to the working directory.
- include_tests: also scan _test.go files. Off by default, since mock secrets and fixtures in tests are normal.

HOW TO READ THE RESULTS:
This is name- and shape-based, not type-checked or data-flow aware -- it can't tell whether a Sprintf's inputs are actually attacker-controlled, and a credential-shaped variable holding a legitimately public value is a false positive. Every finding is a candidate for review, not a confirmed vulnerability; read the surrounding code and confirm exploitability before treating it as a real issue.
