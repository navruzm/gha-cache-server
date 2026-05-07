package tasks

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/navruzm/github-actions-cache-server-go/internal/config"
	dbpkg "github.com/navruzm/github-actions-cache-server-go/internal/db"
	"github.com/navruzm/github-actions-cache-server-go/internal/storage"
	_ "modernc.org/sqlite"
)

func newTestDeps(t *testing.T) Deps {
	t.Helper()
	raw, _ := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	d := &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	a, _ := storage.NewFilesystemAdapter(t.TempDir())
	return Deps{
		Cfg:     &config.Config{CacheCleanupOlderThanDays: 90},
		Queries: q,
		Storage: storage.NewService(q, a, storage.ServiceConfig{}),
	}
}

func TestCleanupUploads_RemovesStale(t *testing.T) {
	ctx := context.Background()
	d := newTestDeps(t)
	old := time.Now().Add(-2 * time.Minute).UnixMilli()
	u, _ := d.Storage.CreateUpload(ctx, "k", "v", "s", "r")
	_ = d.Storage.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_ = d.Queries.SetUploadCreatedAt(ctx, u.ID, old)

	if err := CleanupUploads(d)(ctx); err != nil {
		t.Fatal(err)
	}
	got, _ := d.Queries.FindUploadByID(ctx, u.ID)
	if got != nil {
		t.Errorf("expected upload deleted, got %+v", got)
	}
}
