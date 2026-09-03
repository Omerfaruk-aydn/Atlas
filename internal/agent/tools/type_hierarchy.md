Map which Go types satisfy which interfaces in a tree, without compiling it.

WHEN TO USE THIS TOOL:
- Before changing an interface, to find every type that has to change with it
- When tracing a value through an interface boundary and you need the real implementations behind it
- When reviewing a package's abstractions: which interfaces exist, what they require, who satisfies them

MODES (the `focus` parameter):
- omitted: every interface in the tree with its implementations
- an interface name (e.g. "Service"): the interface's method set and every type that satisfies it
- a type name (e.g. "sqlStore"): that type's methods and every interface it satisfies

HOW IT WORKS:
Files are parsed with go/ast. Method sets are matched by name and by the printed text of the signature, with parameter names ignored. There is no type checking and no build step, so this works on a tree that does not compile.

WHERE IT IS WRONG:
- Interfaces from outside the scanned tree (io.Reader, anything in a dependency) are invisible, so satisfying them is not reported.
- Two identical types spelled differently — an alias, a dot-import, a package qualifier present in one file and not another — will not match, so a real implementation can be missed.
- Generic type parameters are compared as written.

Every one of these errors makes the tool report LESS than the truth, never more: an implementation shown here is real; one absent may still exist. Say so when acting on a "nobody implements this" result.

PARAMETERS:
- path: directory to scan. Defaults to the working directory.
- focus: an interface or type name to narrow the report to. Optional.
- include_tests: also scan _test.go files (finds test fakes and mocks). Default false.

TIPS:
- "*T" in the output means only the pointer satisfies the interface, because at least one method has a pointer receiver. Assigning the value type will not compile.
- Narrow with `focus` before reading; a whole-repository report is long.
