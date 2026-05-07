package tasks

import (
	"context"
	"time"
)

func CleanupUploads(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-time.Minute).UnixMilli()
		for page := 0; ; page++ {
			ups, err := d.Queries.ListStaleUploads(ctx, threshold, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, u := range ups {
				_ = d.Queries.DeleteUpload(ctx, u.ID)
				_ = d.Storage.Adapter().DeleteFolder(ctx, u.FolderName)
			}
			if len(ups) < pageSize {
				return nil
			}
		}
	}
}
