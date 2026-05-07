package tasks

import (
	"context"
	"fmt"
	"time"
)

func CleanupParts(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		for page := 0; ; page++ {
			locs, err := d.Queries.ListMergedNotCleanedStorageLocations(ctx, pageSize, page*pageSize)
			if err != nil {
				return err
			}
			for _, l := range locs {
				if err := d.Queries.UpdateStoragePartsDeleted(ctx, l.ID, time.Now().UnixMilli()); err != nil {
					return err
				}
				if err := d.Storage.Adapter().DeleteFolder(ctx, fmt.Sprintf("%s/parts", l.FolderName)); err != nil {
					return err
				}
			}
			if len(locs) < pageSize {
				return nil
			}
		}
	}
}
