package tasks

import (
	"context"
	"log/slog"
)

var (
	diskPressureBatchSize    = 10
	diskPressureMaxEvictions = 1000
)

func CleanupDiskPressure(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs || d.Cfg.DiskPressureMinFreeBytes <= 0 {
			return nil
		}
		free, ok, err := d.Storage.UsableFreeBytes()
		if err != nil {
			slog.Warn("disk pressure: free-space probe failed", "err", err)
			return nil
		}
		if !ok || free >= d.Cfg.DiskPressureMinFreeBytes {
			return nil
		}
		target := d.Cfg.DiskPressureTargetFreeBytes
		if target < d.Cfg.DiskPressureMinFreeBytes {
			target = d.Cfg.DiskPressureMinFreeBytes
		}
		slog.Warn("disk pressure: starting eviction",
			"free_bytes", free,
			"min_free_bytes", d.Cfg.DiskPressureMinFreeBytes,
			"target_free_bytes", target)

		evicted := 0
		for evicted < diskPressureMaxEvictions {
			entries, err := d.Queries.ListLRUCacheEntries(ctx, diskPressureBatchSize)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				slog.Warn("disk pressure: nothing left to evict",
					"evicted", evicted, "free_bytes", free)
				return nil
			}
			for _, e := range entries {
				if err := d.Queries.DeleteCacheEntry(ctx, e.EntryID); err != nil {
					slog.Warn("disk pressure: delete cache entry failed", "id", e.EntryID, "err", err)
					continue
				}
				if err := d.Queries.DeleteStorageLocation(ctx, e.LocationID); err != nil {
					slog.Warn("disk pressure: delete storage location failed", "id", e.LocationID, "err", err)
				}
				if err := d.Storage.Adapter().DeleteFolder(ctx, e.FolderName); err != nil {
					slog.Warn("disk pressure: delete folder failed", "folder", e.FolderName, "err", err)
				}
				evicted++
			}
			free, _, err = d.Storage.UsableFreeBytes()
			if err != nil {
				return err
			}
			if free >= target {
				slog.Info("disk pressure: target reached",
					"evicted", evicted, "free_bytes", free, "target_free_bytes", target)
				return nil
			}
		}
		slog.Warn("disk pressure: hit max-evictions cap",
			"evicted", evicted, "free_bytes", free, "target_free_bytes", target)
		return nil
	}
}
