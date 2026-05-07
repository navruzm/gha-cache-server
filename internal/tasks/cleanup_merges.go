package tasks

import (
	"context"
	"time"
)

func CleanupMerges(d Deps) func(context.Context) error {
	return func(ctx context.Context) error {
		if d.Cfg.DisableCleanupJobs {
			return nil
		}
		threshold := time.Now().Add(-15 * time.Minute).UnixMilli()
		_, err := d.Queries.ResetStalledMerges(ctx, threshold)
		return err
	}
}
