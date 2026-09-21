package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenDBAppliesPragmas(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	assertStringPragma(t, db, "PRAGMA journal_mode", "wal")
	assertIntPragma(t, db, "PRAGMA busy_timeout", 5000)
	assertIntPragma(t, db, "PRAGMA foreign_keys", 1)
}

func assertStringPragma(t *testing.T, db *sql.DB, query, want string) {
	t.Helper()

	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", query, got, want)
	}
}

func assertIntPragma(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if got != want {
		t.Errorf("%s = %d, want %d", query, got, want)
	}
}
