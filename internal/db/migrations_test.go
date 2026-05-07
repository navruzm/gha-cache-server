package db

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openSQLite(t *testing.T) *DB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	return &DB{DB: raw, Driver: SQLite}
}

func TestMigrate_AppliesAndIsIdempotent(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(ctx, d); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var n int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM cache_entries`).Scan(&n); err != nil {
		t.Fatalf("count cache_entries: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM storage_locations`).Scan(&n); err != nil {
		t.Fatalf("count storage_locations: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM uploads`).Scan(&n); err != nil {
		t.Fatalf("count uploads: %v", err)
	}
}
