package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Queries struct {
	d *DB
}

func New(d *DB) *Queries { return &Queries{d: d} }

func (q *Queries) DB() *DB { return q.d }

func (q *Queries) ph(n int) string { return placeholder(q.d.Driver, n) }

func (q *Queries) phs(n int) string {
	parts := make([]string, n)
	for i := 0; i < n; i++ {
		parts[i] = q.ph(i + 1)
	}
	return strings.Join(parts, ",")
}

func (q *Queries) InsertUpload(ctx context.Context, u Upload) error {
	stmt := fmt.Sprintf(`INSERT INTO uploads
		(id, "key", version, scope, repoId, createdAt, lastPartUploadedAt, folderName,
		 startedPartUploadCount, finishedPartUploadCount)
		VALUES (%s)`, q.phs(10))
	_, err := q.d.ExecContext(ctx, stmt,
		u.ID, u.Key, u.Version, u.Scope, u.RepoID, u.CreatedAt, u.LastPartUploadedAt, u.FolderName,
		u.StartedPartUploadCount, u.FinishedPartUploadCount)
	return err
}

func (q *Queries) FindUploadByKey(ctx context.Context, key, version, scope, repoID string) (*Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName
		FROM uploads WHERE "key"=%s AND version=%s AND scope=%s AND repoId=%s`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	return scanUpload(row)
}

func (q *Queries) FindUploadByID(ctx context.Context, id int64) (*Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName FROM uploads WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanUpload(row)
}

func (q *Queries) IncStartedPartCount(ctx context.Context, id int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET startedPartUploadCount = startedPartUploadCount + 1 WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) IncFinishedPartCount(ctx context.Context, id int64, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET finishedPartUploadCount = finishedPartUploadCount + 1, lastPartUploadedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) DeleteUpload(ctx context.Context, id int64) error {
	stmt := fmt.Sprintf(`DELETE FROM uploads WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) InsertStorageLocation(ctx context.Context, l StorageLocation) error {
	stmt := fmt.Sprintf(`INSERT INTO storage_locations
		(id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt)
		VALUES (%s)`, q.phs(7))
	_, err := q.d.ExecContext(ctx, stmt,
		l.ID, l.FolderName, l.PartCount, l.MergeStartedAt, l.MergedAt, l.PartsDeletedAt, l.LastDownloadedAt)
	return err
}

func (q *Queries) GetStorageLocation(ctx context.Context, id string) (*StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanLocation(row)
}

func (q *Queries) DeleteStorageLocation(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`DELETE FROM storage_locations WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) UpdateStorageLastDownloaded(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET lastDownloadedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStorageMergeStarted(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStorageMerged(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) UpdateStoragePartsDeleted(ctx context.Context, id string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET partsDeletedAt=%s WHERE id=%s`, q.ph(1), q.ph(2))
	_, err := q.d.ExecContext(ctx, stmt, nowMillis, id)
	return err
}

func (q *Queries) ResetStorageMerge(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=NULL, mergedAt=NULL WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) InsertCacheEntry(ctx context.Context, e CacheEntry) error {
	stmt := fmt.Sprintf(`INSERT INTO cache_entries (id, "key", version, scope, repoId, updatedAt, locationId)
		VALUES (%s)`, q.phs(7))
	_, err := q.d.ExecContext(ctx, stmt, e.ID, e.Key, e.Version, e.Scope, e.RepoID, e.UpdatedAt, e.LocationID)
	return err
}

func (q *Queries) UpdateCacheEntryLocation(ctx context.Context, id, locationID string, nowMillis int64) error {
	stmt := fmt.Sprintf(`UPDATE cache_entries SET locationId=%s, updatedAt=%s WHERE id=%s`,
		q.ph(1), q.ph(2), q.ph(3))
	_, err := q.d.ExecContext(ctx, stmt, locationID, nowMillis, id)
	return err
}

func (q *Queries) GetCacheEntry(ctx context.Context, id string) (*CacheEntry, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries WHERE id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	return scanCacheEntry(row)
}

func (q *Queries) DeleteCacheEntry(ctx context.Context, id string) error {
	stmt := fmt.Sprintf(`DELETE FROM cache_entries WHERE id=%s`, q.ph(1))
	_, err := q.d.ExecContext(ctx, stmt, id)
	return err
}

func (q *Queries) FindExactCacheEntry(ctx context.Context, key, version, scope, repoID string) (*CacheEntry, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries
		WHERE "key"=%s AND version=%s AND scope=%s AND repoId=%s
		ORDER BY updatedAt DESC LIMIT 1`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	return scanCacheEntry(row)
}

func (q *Queries) FindPrefixedCacheEntry(ctx context.Context, keyPrefix, version, scope, repoID string) (*CacheEntry, error) {
	pattern := escapeLike(keyPrefix) + "%"
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries
		WHERE "key" LIKE %s ESCAPE '\' AND version=%s AND scope=%s AND repoId=%s
		ORDER BY updatedAt DESC LIMIT 1`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, pattern, version, scope, repoID)
	return scanCacheEntry(row)
}

func (q *Queries) FindCacheEntryWithLocation(ctx context.Context, id string) (*CacheEntry, *StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT c.id, c."key", c.version, c.scope, c.repoId, c.updatedAt, c.locationId,
		s.id, s.folderName, s.partCount, s.mergeStartedAt, s.mergedAt, s.partsDeletedAt, s.lastDownloadedAt
		FROM cache_entries c JOIN storage_locations s ON s.id = c.locationId
		WHERE c.id=%s`, q.ph(1))
	row := q.d.QueryRowContext(ctx, stmt, id)
	var e CacheEntry
	var l StorageLocation
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID,
		&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &e, &l, nil
}

func (q *Queries) FindExistingCacheEntryWithLocation(ctx context.Context, key, version, scope, repoID string) (*CacheEntry, *StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT c.id, c."key", c.version, c.scope, c.repoId, c.updatedAt, c.locationId,
		s.id, s.folderName, s.partCount, s.mergeStartedAt, s.mergedAt, s.partsDeletedAt, s.lastDownloadedAt
		FROM cache_entries c JOIN storage_locations s ON s.id = c.locationId
		WHERE c."key"=%s AND c.version=%s AND c.scope=%s AND c.repoId=%s`,
		q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	row := q.d.QueryRowContext(ctx, stmt, key, version, scope, repoID)
	var e CacheEntry
	var l StorageLocation
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID,
		&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &e, &l, nil
}

func (q *Queries) ListExpiredStorageLocations(ctx context.Context, beforeMillis int64, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE lastDownloadedAt < %s
		LIMIT %s OFFSET %s`,
		q.ph(1), q.ph(2), q.ph(3))
	return q.queryLocations(ctx, stmt, beforeMillis, limit, offset)
}

func (q *Queries) ListOrphanedStorageLocations(ctx context.Context, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations s
		WHERE NOT EXISTS (SELECT 1 FROM cache_entries c WHERE c.locationId = s.id)
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2))
	return q.queryLocations(ctx, stmt, limit, offset)
}

func (q *Queries) ListMergedNotCleanedStorageLocations(ctx context.Context, limit, offset int) ([]StorageLocation, error) {
	stmt := fmt.Sprintf(`SELECT id, folderName, partCount, mergeStartedAt, mergedAt, partsDeletedAt, lastDownloadedAt
		FROM storage_locations WHERE mergedAt IS NOT NULL AND partsDeletedAt IS NULL
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2))
	return q.queryLocations(ctx, stmt, limit, offset)
}

func (q *Queries) ResetStalledMerges(ctx context.Context, beforeMillis int64) (int64, error) {
	stmt := fmt.Sprintf(`UPDATE storage_locations SET mergeStartedAt=NULL, mergedAt=NULL
		WHERE mergeStartedAt < %s AND mergedAt IS NULL`, q.ph(1))
	res, err := q.d.ExecContext(ctx, stmt, beforeMillis)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (q *Queries) ListStaleUploads(ctx context.Context, beforeMillis int64, limit, offset int) ([]Upload, error) {
	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, createdAt, lastPartUploadedAt,
		startedPartUploadCount, finishedPartUploadCount, folderName
		FROM uploads
		WHERE (lastPartUploadedAt IS NULL OR lastPartUploadedAt < %s) AND createdAt < %s
		LIMIT %s OFFSET %s`, q.ph(1), q.ph(2), q.ph(3), q.ph(4))
	rows, err := q.d.QueryContext(ctx, stmt, beforeMillis, beforeMillis, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Upload
	for rows.Next() {
		u, err := scanUploadRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (q *Queries) queryLocations(ctx context.Context, stmt string, args ...any) ([]StorageLocation, error) {
	rows, err := q.d.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StorageLocation
	for rows.Next() {
		var l StorageLocation
		if err := rows.Scan(&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

type Tx struct {
	*sql.Tx
	q *Queries
}

func (q *Queries) BeginTx(ctx context.Context) (*Tx, error) {
	t, err := q.d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: t, q: q}, nil
}

func scanUpload(row *sql.Row) (*Upload, error) {
	var u Upload
	err := row.Scan(&u.ID, &u.Key, &u.Version, &u.Scope, &u.RepoID, &u.CreatedAt, &u.LastPartUploadedAt,
		&u.StartedPartUploadCount, &u.FinishedPartUploadCount, &u.FolderName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func scanUploadRows(rows *sql.Rows) (*Upload, error) {
	var u Upload
	err := rows.Scan(&u.ID, &u.Key, &u.Version, &u.Scope, &u.RepoID, &u.CreatedAt, &u.LastPartUploadedAt,
		&u.StartedPartUploadCount, &u.FinishedPartUploadCount, &u.FolderName)
	return &u, err
}

func scanLocation(row *sql.Row) (*StorageLocation, error) {
	var l StorageLocation
	err := row.Scan(&l.ID, &l.FolderName, &l.PartCount, &l.MergeStartedAt, &l.MergedAt, &l.PartsDeletedAt, &l.LastDownloadedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &l, err
}

func scanCacheEntry(row *sql.Row) (*CacheEntry, error) {
	var e CacheEntry
	err := row.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &e, err
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

func (q *Queries) SetUploadCreatedAt(ctx context.Context, id, t int64) error {
	stmt := fmt.Sprintf(`UPDATE uploads SET createdAt=%s, lastPartUploadedAt=%s WHERE id=%s`,
		q.ph(1), q.ph(2), q.ph(3))
	_, err := q.d.ExecContext(ctx, stmt, t, t, id)
	return err
}

type CacheEntryFilter struct {
	Key, Version, Scope, RepoID string
}

func (q *Queries) ListCacheEntries(ctx context.Context, f CacheEntryFilter, limit, offset int) ([]CacheEntry, int, error) {
	conds, args := []string{"1=1"}, []any{}
	add := func(field, val string) {
		if val == "" {
			return
		}
		conds = append(conds, fmt.Sprintf(`%s=%s`, field, q.ph(len(args)+1)))
		args = append(args, val)
	}
	add(`"key"`, f.Key)
	add(`version`, f.Version)
	add(`scope`, f.Scope)
	add(`repoId`, f.RepoID)
	where := strings.Join(conds, " AND ")

	countStmt := `SELECT count(*) FROM cache_entries WHERE ` + where
	var total int
	if err := q.d.QueryRowContext(ctx, countStmt, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	stmt := fmt.Sprintf(`SELECT id, "key", version, scope, repoId, updatedAt, locationId
		FROM cache_entries WHERE %s ORDER BY updatedAt DESC LIMIT %s OFFSET %s`,
		where, q.ph(len(args)+1), q.ph(len(args)+2))
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, limit, offset)
	rows, err := q.d.QueryContext(ctx, stmt, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []CacheEntry
	for rows.Next() {
		var e CacheEntry
		if err := rows.Scan(&e.ID, &e.Key, &e.Version, &e.Scope, &e.RepoID, &e.UpdatedAt, &e.LocationID); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (q *Queries) DeleteCacheEntries(ctx context.Context, f CacheEntryFilter) (int64, error) {
	conds, args := []string{"1=1"}, []any{}
	add := func(field, val string) {
		if val == "" {
			return
		}
		conds = append(conds, fmt.Sprintf(`%s=%s`, field, q.ph(len(args)+1)))
		args = append(args, val)
	}
	add(`"key"`, f.Key)
	add(`version`, f.Version)
	add(`scope`, f.Scope)
	add(`repoId`, f.RepoID)
	where := strings.Join(conds, " AND ")
	res, err := q.d.ExecContext(ctx, `DELETE FROM cache_entries WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type LRUCacheEntry struct {
	EntryID    string
	LocationID string
	FolderName string
}

func (q *Queries) ListLRUCacheEntries(ctx context.Context, limit int) ([]LRUCacheEntry, error) {
	stmt := fmt.Sprintf(`SELECT c.id, s.id, s.folderName
		FROM cache_entries c JOIN storage_locations s ON s.id = c.locationId
		WHERE NOT (s.mergeStartedAt IS NOT NULL AND s.mergedAt IS NULL)
		ORDER BY COALESCE(s.lastDownloadedAt, c.updatedAt) ASC
		LIMIT %s`, q.ph(1))
	rows, err := q.d.QueryContext(ctx, stmt, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LRUCacheEntry
	for rows.Next() {
		var e LRUCacheEntry
		if err := rows.Scan(&e.EntryID, &e.LocationID, &e.FolderName); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
