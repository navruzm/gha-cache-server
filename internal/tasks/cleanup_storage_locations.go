package tasks

import "context"

func CleanupStorageLocations(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		for page := 0; ; page++ {
			locs, err := d.Queries.ListOrphanedStorageLocations(ctx, pageSize, page*pageSize)
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
