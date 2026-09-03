List what a Go package exposes to its callers — the exported functions, types, methods, fields, constants and variables.

WHEN TO USE THIS TOOL:
- Learning an unfamiliar package: the exported surface is what it is *for*, and it is usually a small fraction of what it contains
- Before renaming or changing a signature, to see whether you are touching the contract or the implementation
- Before a release, to check what has been added to the public API — additions are permanent in a way internals are not
- Finding undocumented exported symbols
- Finding what is already marked Deprecated, before adding to it

WHAT COUNTS AS THE SURFACE:
Exported functions and types, methods on exported types, and — importantly — exported fields of exported structs. A field is part of the contract exactly as much as a method: changing its type breaks callers the same way. Embedded fields are listed too, since they promote a whole method set.

A capitalised method on an unexported type is NOT included: nothing outside the package can reach it.

WHAT YOU GET PER SYMBOL:
The rendered signature, the first line of its doc comment, and whether it carries a `Deprecated:` marker. Only the first line of documentation — Go convention puts the summary there, and reprinting whole doc comments would make the listing longer than reading the source.

Types are listed with their own fields and methods grouped underneath rather than scattered alphabetically.

PARAMETERS:
- path: directory to scan. Defaults to the working directory. Point it at one package for a readable answer.
- package: restrict to a package by name or directory suffix.
- undocumented_only: list only exported symbols with no doc comment.
- include_tests: also scan _test.go files. Default false.

NOTE:
Type declarations show the shape (`type Config struct`) rather than the full body — a large struct's body is the whole file. Read the file itself when you need the fields' types in context.
