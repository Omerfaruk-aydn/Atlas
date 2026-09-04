package datafile

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestSQLiteDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.db")

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, "weird""col" TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob'), (3, NULL)`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIsSQLite(t *testing.T) {
	path := newTestSQLiteDB(t)
	if !IsSQLite(path) {
		t.Error("expected real sqlite db to be detected")
	}

	notDB := filepath.Join(t.TempDir(), "plain.txt")
	if err := os.WriteFile(notDB, []byte("hello world, not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsSQLite(notDB) {
		t.Error("expected plain text file to not be detected as sqlite")
	}

	tooShort := filepath.Join(t.TempDir(), "short")
	if err := os.WriteFile(tooShort, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsSQLite(tooShort) {
		t.Error("expected too-short file to not be detected as sqlite")
	}
}

func TestInspectSQLiteSchema(t *testing.T) {
	path := newTestSQLiteDB(t)

	tables, err := InspectSQLiteSchema(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 {
		t.Fatalf("expected 1 table, got %d: %+v", len(tables), tables)
	}
	if tables[0].Name != "users" {
		t.Fatalf("expected table 'users', got %q", tables[0].Name)
	}
	wantCols := map[string]bool{"id": true, "name": true, `weird"col`: true}
	if len(tables[0].Columns) != len(wantCols) {
		t.Fatalf("unexpected columns: %+v", tables[0].Columns)
	}
	for _, c := range tables[0].Columns {
		if !wantCols[c] {
			t.Errorf("unexpected column %q", c)
		}
	}
}

func TestQuerySQLite(t *testing.T) {
	path := newTestSQLiteDB(t)

	result, err := QuerySQLite(context.Background(), path, "SELECT id, name FROM users ORDER BY id", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %+v", result.Columns)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result.Rows))
	}
	if result.Truncated {
		t.Error("expected not truncated")
	}
	if result.Rows[2][1] != "NULL" {
		t.Errorf("expected NULL rendering, got %q", result.Rows[2][1])
	}
}

func TestQuerySQLiteLimitTruncates(t *testing.T) {
	path := newTestSQLiteDB(t)

	result, err := QuerySQLite(context.Background(), path, "SELECT id FROM users ORDER BY id", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result.Rows))
	}
	if !result.Truncated {
		t.Error("expected truncated")
	}
}

func TestQuerySQLiteRejectsWrite(t *testing.T) {
	path := newTestSQLiteDB(t)

	if _, err := QuerySQLite(context.Background(), path, "DELETE FROM users", 0); err == nil {
		t.Fatal("expected write query to fail against a read-only connection")
	}

	// Confirm nothing was actually deleted.
	result, err := QuerySQLite(context.Background(), path, "SELECT id FROM users", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("expected rows to remain untouched, got %d", len(result.Rows))
	}
}
