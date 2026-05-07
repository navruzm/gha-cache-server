package tasks

import (
	"context"
	"time"
)

func CleanupCacheEntries(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-time.Duration(d.Cfg.CacheCleanupOlderThanDays) * 24 * time.Hour).UnixMilli()
		for page := 0; ; page++ {
			locs, err := d.Queries.ListExpiredStorageLocations(ctx, threshold, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, l := range locs {
				_ = d.Queries.DeleteStorageLocation(ctx, l.ID)
				_ = d.Storage.Adapter().DeleteFolder(ctx, l.FolderName)
			}
			if len(locs) < pageSize {
				return nil
			}
		}
	}
}
