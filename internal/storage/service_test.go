package storage

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"testing"

	dbpkg "github.com/navruzm/gha-cache-server/internal/db"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *dbpkg.DB {
	t.Helper()
	raw, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	raw.SetMaxOpenConns(1)
	return &dbpkg.DB{DB: raw, Driver: dbpkg.SQLite}
}

func newServiceForTest(t *testing.T) (*Service, *dbpkg.Queries) {
	t.Helper()
	d := openTestDB(t)
	if err := dbpkg.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	q := dbpkg.New(d)
	dir := t.TempDir()
	a, err := NewFilesystemAdapter(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(q, a, ServiceConfig{APIBaseURL: "http://localhost:3000"}), q
}

func TestCreateUpload_NewAndIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, err := svc.CreateUpload(ctx, "k", "v", "s", "r")
	if err != nil || u == nil {
		t.Fatalf("CreateUpload: %+v err=%v", u, err)
	}
	dup, err := svc.CreateUpload(ctx, "k", "v", "s", "r")
	if err != nil {
		t.Fatalf("dup CreateUpload: %v", err)
	}
	if dup != nil {
		t.Errorf("expected nil for existing upload, got %+v", dup)
	}
}

func TestUploadPart_StoresPartAndUpdatesCounters(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")

	if err := svc.UploadPart(ctx, u.ID, 0, strings.NewReader("part-zero")); err != nil {
		t.Fatalf("UploadPart: %v", err)
	}

	got, _ := q.FindUploadByID(ctx, u.ID)
	if got.StartedPartUploadCount != 1 || got.FinishedPartUploadCount != 1 {
		t.Errorf("counters: %+v", got)
	}

	r, err := svc.adapter.CreateDownloadStream(ctx, fmt.Sprintf("%s/parts/0", u.FolderName))
	if err != nil {
		t.Fatalf("download part: %v", err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "part-zero" {
		t.Errorf("got %q", body)
	}
}

func TestUploadPart_UnknownUploadIsNoop(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	if err := svc.UploadPart(ctx, 9999999999, 0, strings.NewReader("x")); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestCompleteUpload_NewEntry(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("hello"))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("world"))

	got, err := svc.CompleteUpload(ctx, "k", "v", "s", "r")
	if err != nil {
		t.Fatalf("CompleteUpload: %v", err)
	}
	if got == nil {
		t.Fatal("expected upload returned")
	}

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	if entry == nil {
		t.Fatal("expected cache entry")
	}
	loc, _ := q.GetStorageLocation(ctx, entry.LocationID)
	if loc == nil || loc.PartCount != 2 {
		t.Errorf("loc=%+v", loc)
	}
}

func TestCompleteUpload_ReplacesExisting(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)

	u1, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u1.ID, 0, strings.NewReader("v1"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	u2, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u2.ID, 0, strings.NewReader("v2"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	if entry == nil {
		t.Fatal("expected cache entry")
	}
}

func TestCompleteUpload_ZeroPartsRejected(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	_, _ = svc.CreateUpload(ctx, "k", "v", "s", "r")
	_, err := svc.CompleteUpload(ctx, "k", "v", "s", "r")
	if err == nil {
		t.Fatal("expected error when 0 parts uploaded")
	}
}

func TestDownload_FirstReadStreamsAndMerges(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("AAA"))
	_ = svc.UploadPart(ctx, u.ID, 1, strings.NewReader("BBB"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	r, err := svc.Download(ctx, entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, _ := io.ReadAll(r)
	if string(body) != "AAABBB" {
		t.Errorf("got %q", body)
	}
	svc.WaitForOngoingMerges(ctx)
	merged, err := svc.adapter.CreateDownloadStream(ctx, u.FolderName+"/merged")
	if err != nil {
		t.Fatalf("merged not present: %v", err)
	}
	defer merged.Close()
	mb, _ := io.ReadAll(merged)
	if string(mb) != "AAABBB" {
		t.Errorf("merged got %q", mb)
	}
}

func TestDownload_NotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	r, err := svc.Download(ctx, "nope")
	if err != nil {
		t.Fatalf("expected nil error for missing entry, got %v", err)
	}
	if r != nil {
		t.Error("expected nil reader for missing entry")
	}
}

func TestDownload_StaleParts_ReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc, q := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "k", "v", "s", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("X"))
	_, _ = svc.CompleteUpload(ctx, "k", "v", "s", "r")
	_ = svc.adapter.Clear(ctx)

	entry, _ := q.FindExactCacheEntry(ctx, "k", "v", "s", "r")
	r, err := svc.Download(ctx, entry.ID)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if r != nil {
		t.Error("expected nil reader for stale entry")
	}
}

func TestMatchCacheEntry_PrefersExactPrimary(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	mustSave := func(key, val string) {
		t.Helper()
		u, _ := svc.CreateUpload(ctx, key, "v", "main", "r")
		_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader(val))
		_, err := svc.CompleteUpload(ctx, key, "v", "main", "r")
		if err != nil {
			t.Fatal(err)
		}
	}
	mustSave("deps-abc", "1")
	mustSave("deps-xyz", "2")

	got, err := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"deps-abc"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if err != nil || got == nil {
		t.Fatalf("got %+v err=%v", got, err)
	}
	if got.Type != MatchExactPrimary || got.Entry.Key != "deps-abc" {
		t.Errorf("got %+v", got)
	}
}

func TestMatchCacheEntry_PrefixedPrimary(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "deps-abc", "v", "main", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "deps-abc", "v", "main", "r")

	got, _ := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"deps-"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if got == nil || got.Type != MatchPrefixedPrimary {
		t.Errorf("got %+v", got)
	}
}

func TestMatchCacheEntry_FallsBackToRestoreKey(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceForTest(t)
	u, _ := svc.CreateUpload(ctx, "deps-abc", "v", "main", "r")
	_ = svc.UploadPart(ctx, u.ID, 0, strings.NewReader("x"))
	_, _ = svc.CompleteUpload(ctx, "deps-abc", "v", "main", "r")

	got, _ := svc.MatchCacheEntry(ctx, MatchInput{
		Keys: []string{"missing-key", "deps-abc"}, Version: "v", Scopes: []string{"main"}, RepoID: "r",
	})
	if got == nil || got.Type != MatchExactRestore {
		t.Errorf("got %+v", got)
	}
}
