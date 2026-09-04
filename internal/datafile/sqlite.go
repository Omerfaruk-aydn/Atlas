package datafile

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/Omerfaruk-aydn/Atlas-Agent/internal/db"
)

// TableInfo is one table's name and column list.
type TableInfo struct {
	Name    string
	Columns []string
}

// QueryResult is a query's column names and rows, each cell rendered as
// text -- good enough for display, not for round-tripping a value's
// original type.
type QueryResult struct {
	Columns []string
	Rows    [][]string
	// Truncated reports that more rows matched than Limit allowed.
	Truncated bool
}

// InspectSQLiteSchema opens path read-only and lists every table with
// its columns.
//
// The connection is opened in SQLite's own mode=ro, a VFS-level
// restriction on the primary file -- not just an application-level
// check -- via internal/db.ConnectReadOnly, the same helper Atlas's own
// session database uses for read-only access.
func InspectSQLiteSchema(ctx context.Context, path string) ([]TableInfo, error) {
	conn, err := db.ConnectReadOnly(ctx, path)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("reading schema: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]TableInfo, 0, len(tables))
	for _, name := range tables {
		cols, err := tableColumns(ctx, conn, name)
		if err != nil {
			return nil, err
		}
		result = append(result, TableInfo{Name: name, Columns: cols})
	}
	return result, nil
}

func tableColumns(ctx context.Context, conn *sql.DB, table string) ([]string, error) {
	// PRAGMA doesn't accept bound parameters; the table name comes from
	// sqlite_master itself, not from external input. It is still quoted
	// as a SQL identifier (doubled internal quotes, not Go's %q escaping,
	// which a name containing a literal quote character would need).
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, strings.ReplaceAll(table, `"`, `""`)))
	if err != nil {
		return nil, fmt.Errorf("reading columns for %s: %w", table, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	nameIdx := -1
	for i, c := range cols {
		if c == "name" {
			nameIdx = i
		}
	}
	if nameIdx < 0 {
		return nil, fmt.Errorf("unexpected PRAGMA table_info shape for %s", table)
	}

	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		if name, ok := vals[nameIdx].(string); ok {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// QuerySQLite runs a read-only query against path and returns up to
// limit rows. The connection itself is opened read-only at the SQLite
// level (see InspectSQLiteSchema), so a write statement fails there
// rather than being screened by string matching here, which is far
// easier to get wrong or bypass.
func QuerySQLite(ctx context.Context, path, query string, limit int) (QueryResult, error) {
	if limit <= 0 {
		limit = 50
	}

	conn, err := db.ConnectReadOnly(ctx, path)
	if err != nil {
		return QueryResult{}, err
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return QueryResult{}, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return QueryResult{}, err
	}

	result := QueryResult{Columns: cols}
	for rows.Next() {
		if len(result.Rows) >= limit {
			result.Truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return QueryResult{}, err
		}
		row := make([]string, len(cols))
		for i, v := range vals {
			row[i] = renderCell(v)
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

func renderCell(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// IsSQLite sniffs the standard 16-byte SQLite file header, since a
// database file's extension is a convention (.db, .sqlite, .sqlite3),
// not a guarantee.
func IsSQLite(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 16)
	n, err := f.Read(header)
	if err != nil || n < 16 {
		return false
	}
	return string(header) == "SQLite format 3\x00"
}
