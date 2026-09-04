Generate a table-driven test skeleton for one package-level Go function, shaped from its own signature.

WHEN TO USE THIS TOOL:
- A function was just written or changed and has no test yet
- Starting a test for an existing function without hand-writing the table struct and the `t.Run` loop boilerplate every time

WHAT IT PRODUCES:
A `TestXxx` function with a struct-of-cases table: one field per parameter (using the parameter's own name and type), a `want` field for a single non-error return value, and a `wantErr bool` field when the function returns an error. The call inside `t.Run` is built to match: `err := Foo(tt.a, tt.b)` plus a `require.Equal` assertion, or `got, err := Foo(...)` with the assertion guarded by `!tt.wantErr` when there's a value and an error. A lone `context.Context` parameter is passed `context.Background()` directly rather than becoming a table field -- there is rarely a reason to vary it case by case.

WHAT YOU STILL HAVE TO DO:
The generated table has exactly one row, `{name: "TODO"}`, with every other field at its zero value. Fill in real cases -- this tool has no way to know what inputs are interesting for this specific function. The `imports` list in the response says what to add to the test file (`testing`, `github.com/stretchr/testify/require`, and `context` when relevant) -- this is a snippet to paste into an existing or new _test.go file, not a complete file with its own package and import block.

PARAMETERS:
- path: directory or single file to search. Defaults to the working directory.
- symbol: the function name to generate a test for. Required.

WHAT THIS DOES NOT DO:
It only supports package-level functions, not methods -- constructing a receiver generically isn't possible from syntax alone, and guessing wrong (a zero value, say) would produce a skeleton that compiles but tests nothing real. A function with more than two return values gets a call with a TODO comment instead of struct fields, since guessing which of several values matters isn't something syntax can answer either.
