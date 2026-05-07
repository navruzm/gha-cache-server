package tasks

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/navruzm/gha-cache-server/internal/storage"
)

type fakeFreeReporter struct {
	*storage.FilesystemAdapter
	free       atomic.Int64
	growOnEach atomic.Int64
}

func (f *fakeFreeReporter) UsableFreeBytes() (int64, error) {
	cur := f.free.Load()
	step := f.growOnEach.Load()
	if step != 0 {
		f.free.Add(step)
	}
	return cur, nil
}

func newPressureDeps(t *testing.T) (Deps, *fakeFreeReporter) {
	t.Helper()
	d := newTestDeps(t)
	fs := d.Storage.Adapter().(*storage.FilesystemAdapter)
	r := &fakeFreeReporter{FilesystemAdapter: fs}
	r.free.Store(1 << 40)
	d.Storage = storage.NewService(d.Queries, r, storage.ServiceConfig{})
	return d, r
}

func saveEntry(t *testing.T, d Deps, key string) string {
	t.Helper()
	ctx := context.Background()
	u, err := d.Storage.CreateUpload(ctx, key, "v", "s", "r")
	if err != nil || u == nil {
		t.Fatalf("CreateUpload(%s): %+v err=%v", key, u, err)
	}
	if err := d.Storage.UploadPart(ctx, u.ID, 0, strings.NewReader("X")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Storage.CompleteUpload(ctx, key, "v", "s", "r"); err != nil {
		t.Fatal(err)
	}
	got, err := d.Queries.FindExactCacheEntry(ctx, key, "v", "s", "r")
	if err != nil || got == nil {
		t.Fatalf("FindExactCacheEntry(%s): %+v err=%v", key, got, err)
	}
	return got.ID
}

func TestCleanupDiskPressure_DisabledWhenMinFreeZero(t *testing.T) {
	ctx := context.Background()
	d, r := newPressureDeps(t)
	d.Cfg.DiskPressureMinFreeBytes = 0
	r.free.Store(0)
	saveEntry(t, d, "k")

	if err := CleanupDiskPressure(d)(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Queries.FindExactCacheEntry(ctx, "k", "v", "s", "r"); got == nil {
		t.Errorf("entry should not be evicted when min_free is 0")
	}
}

func TestCleanupDiskPressure_NoOpAboveThreshold(t *testing.T) {
	ctx := context.Background()
	d, r := newPressureDeps(t)
	d.Cfg.DiskPressureMinFreeBytes = 1 << 30
	d.Cfg.DiskPressureTargetFreeBytes = 2 << 30
	r.free.Store(10 << 30)
	saveEntry(t, d, "k")

	if err := CleanupDiskPressure(d)(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Queries.FindExactCacheEntry(ctx, "k", "v", "s", "r"); got == nil {
		t.Errorf("entry evicted despite plenty of free space")
	}
}

func TestCleanupDiskPressure_NoReporterIsNoOp(t *testing.T) {
	ctx := context.Background()
	d := newTestDeps(t)
	d.Cfg.DiskPressureMinFreeBytes = 1 << 30
	d.Cfg.DiskPressureTargetFreeBytes = 2 << 30

	saveEntry(t, d, "k")
	if err := CleanupDiskPressure(d)(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.Queries.FindExactCacheEntry(ctx, "k", "v", "s", "r"); got == nil {
		t.Errorf("entry evicted but adapter doesn't report disk usage")
	}
}

func TestCleanupDiskPressure_EvictsLRUFirst(t *testing.T) {
	t.Cleanup(func() { diskPressureBatchSize = 10 })
	diskPressureBatchSize = 1

	ctx := context.Background()
	d, r := newPressureDeps(t)
	d.Cfg.DiskPressureMinFreeBytes = 100
	d.Cfg.DiskPressureTargetFreeBytes = 200

	idA := saveEntry(t, d, "a")
	idB := saveEntry(t, d, "b")
	idC := saveEntry(t, d, "c")

	entryA, _ := d.Queries.GetCacheEntry(ctx, idA)
	entryB, _ := d.Queries.GetCacheEntry(ctx, idB)
	entryC, _ := d.Queries.GetCacheEntry(ctx, idC)
	if err := d.Queries.UpdateStorageLastDownloaded(ctx, entryA.LocationID, 100); err != nil {
		t.Fatal(err)
	}
	if err := d.Queries.UpdateStorageLastDownloaded(ctx, entryB.LocationID, 200); err != nil {
		t.Fatal(err)
	}
	if err := d.Queries.UpdateStorageLastDownloaded(ctx, entryC.LocationID, 999_999_999); err != nil {
		t.Fatal(err)
	}

	r.free.Store(50)
	r.growOnEach.Store(80)

	if err := CleanupDiskPressure(d)(ctx); err != nil {
		t.Fatal(err)
	}

	if got, _ := d.Queries.GetCacheEntry(ctx, idA); got != nil {
		t.Errorf("oldest LRU entry A should have been evicted first")
	}
	if got, _ := d.Queries.GetCacheEntry(ctx, idC); got == nil {
		t.Errorf("most-recently-used entry C should have survived")
	}
}

func TestCleanupDiskPressure_StopsAtTarget(t *testing.T) {
	t.Cleanup(func() { diskPressureBatchSize = 10 })
	diskPressureBatchSize = 1

	ctx := context.Background()
	d, r := newPressureDeps(t)
	d.Cfg.DiskPressureMinFreeBytes = 100
	d.Cfg.DiskPressureTargetFreeBytes = 200

	keys := []string{"a", "b", "c", "d", "e", "f"}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = saveEntry(t, d, k)
	}
	for i, id := range ids {
		e, _ := d.Queries.GetCacheEntry(ctx, id)
		if err := d.Queries.UpdateStorageLastDownloaded(ctx, e.LocationID, int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}

	r.free.Store(50)
	r.growOnEach.Store(40)

	if err := CleanupDiskPressure(d)(ctx); err != nil {
		t.Fatal(err)
	}

	survivors := 0
	for _, id := range ids {
		if got, _ := d.Queries.GetCacheEntry(ctx, id); got != nil {
			survivors++
		}
	}
	if survivors == 0 {
		t.Errorf("expected eviction to stop early, got 0 survivors out of %d", len(ids))
	}
	if survivors == len(ids) {
		t.Errorf("expected at least one eviction, got %d survivors", survivors)
	}
}
