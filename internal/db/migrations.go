package db

import (
	"context"
	"errors"
	"fmt"
)

type migration struct {
	name string
	fn   func(ctx context.Context, d *DB) error
}

var migrations = []migration{
	{name: "0_init", fn: m0Init},
}

func Migrate(ctx context.Context, d *DB) error {
	if err := ensureMigrationsTable(ctx, d); err != nil {
		return err
	}
	applied, err := loadApplied(ctx, d)
	if err != nil {
		return err
	}
	for _, m := range migrations {
		if applied[m.name] {
			continue
		}
		if err := m.fn(ctx, d); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := d.ExecContext(ctx, `INSERT INTO _migrations(name) VALUES (`+placeholder(d.Driver, 1)+`)`, m.name); err != nil {
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, d *DB) error {
	stmt := `CREATE TABLE IF NOT EXISTS _migrations (name TEXT PRIMARY KEY)`
	if d.Driver == MySQL {
		stmt = `CREATE TABLE IF NOT EXISTS _migrations (name VARCHAR(64) PRIMARY KEY)`
	}
	_, err := d.ExecContext(ctx, stmt)
	return err
}

func loadApplied(ctx context.Context, d *DB) (map[string]bool, error) {
	rows, err := d.QueryContext(ctx, `SELECT name FROM _migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

func idType(d *DB) string {
	if d.Driver == MySQL {
		return "VARCHAR(36)"
	}
	return "TEXT"
}

func textType(d *DB, max int) string {
	if d.Driver == MySQL {
		return fmt.Sprintf("VARCHAR(%d)", max)
	}
	return "TEXT"
}

func placeholder(drv Driver, n int) string {
	if drv == Postgres {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

func m0Init(ctx context.Context, d *DB) error {
	idCol := idType(d)
	keyCol := textType(d, 512)
	versionCol := textType(d, 255)
	scopeCol := textType(d, 255)
	repoIDCol := textType(d, 255)

	stmts := []string{
		fmt.Sprintf(`CREATE TABLE storage_locations (
			id %s PRIMARY KEY,
			folderName TEXT NOT NULL,
			partCount INTEGER NOT NULL,
			mergeStartedAt BIGINT,
			mergedAt BIGINT,
			partsDeletedAt BIGINT,
			lastDownloadedAt BIGINT
		)`, idCol),
		fmt.Sprintf(`CREATE TABLE cache_entries (
			id %s PRIMARY KEY,
			"key" %s NOT NULL,
			version %s NOT NULL,
			scope %s NOT NULL,
			repoId %s NOT NULL,
			updatedAt BIGINT NOT NULL,
			locationId %s NOT NULL REFERENCES storage_locations(id) ON DELETE CASCADE
		)`, idCol, keyCol, versionCol, scopeCol, repoIDCol, idCol),
		fmt.Sprintf(`CREATE TABLE uploads (
			id BIGINT PRIMARY KEY,
			"key" %s NOT NULL,
			version %s NOT NULL,
			scope %s NOT NULL,
			repoId %s NOT NULL,
			createdAt BIGINT NOT NULL,
			lastPartUploadedAt BIGINT,
			startedPartUploadCount INTEGER NOT NULL DEFAULT 0,
			finishedPartUploadCount INTEGER NOT NULL DEFAULT 0,
			folderName TEXT NOT NULL
		)`, keyCol, versionCol, scopeCol, repoIDCol),
		`CREATE INDEX idx_cache_entries_key_version ON cache_entries("key", version)`,
		`CREATE INDEX idx_cache_entries_scope ON cache_entries(scope)`,
		`CREATE INDEX idx_cache_entries_repoId ON cache_entries(repoId)`,
		`CREATE INDEX idx_uploads_key_version ON uploads("key", version)`,
		`CREATE INDEX idx_uploads_scope ON uploads(scope)`,
		`CREATE INDEX idx_uploads_repoId ON uploads(repoId)`,
	}
	for _, s := range stmts {
		if _, err := d.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

var ErrNoRowsAffected = errors.New("no rows affected")
