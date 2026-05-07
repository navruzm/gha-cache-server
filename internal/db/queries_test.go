package db

import (
	"context"
	"testing"
)

func TestQueries_UploadLifecycle(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	q := New(d)

	if err := q.InsertUpload(ctx, Upload{
		ID: 1234567890, Key: "k", Version: "v", Scope: "s", RepoID: "r",
		CreatedAt: 1, FolderName: "1234567890",
	}); err != nil {
		t.Fatalf("InsertUpload: %v", err)
	}

	got, err := q.FindUploadByKey(ctx, "k", "v", "s", "r")
	if err != nil || got == nil || got.ID != 1234567890 {
		t.Fatalf("FindUploadByKey: %+v err=%v", got, err)
	}

	if err := q.IncStartedPartCount(ctx, 1234567890); err != nil {
		t.Fatalf("IncStartedPartCount: %v", err)
	}
	if err := q.IncFinishedPartCount(ctx, 1234567890, 42); err != nil {
		t.Fatalf("IncFinishedPartCount: %v", err)
	}

	got, _ = q.FindUploadByID(ctx, 1234567890)
	if got.StartedPartUploadCount != 1 || got.FinishedPartUploadCount != 1 {
		t.Errorf("counts: %+v", got)
	}
}

func TestQueries_CacheEntryMatch(t *testing.T) {
	d := openSQLite(t)
	ctx := context.Background()
	if err := Migrate(ctx, d); err != nil {
		t.Fatal(err)
	}
	q := New(d)

	if err := q.InsertStorageLocation(ctx, StorageLocation{ID: "loc1", FolderName: "f1", PartCount: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertCacheEntry(ctx, CacheEntry{
		ID: "e1", Key: "deps-abc", Version: "v1", Scope: "main", RepoID: "1", UpdatedAt: 100, LocationID: "loc1",
	}); err != nil {
		t.Fatal(err)
	}

	got, err := q.FindExactCacheEntry(ctx, "deps-abc", "v1", "main", "1")
	if err != nil || got == nil {
		t.Fatalf("FindExactCacheEntry: %+v err=%v", got, err)
	}

	got, err = q.FindPrefixedCacheEntry(ctx, "deps-", "v1", "main", "1")
	if err != nil || got == nil || got.ID != "e1" {
		t.Fatalf("FindPrefixedCacheEntry: %+v err=%v", got, err)
	}
}
