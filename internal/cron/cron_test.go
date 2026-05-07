package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerFiresAndStops(t *testing.T) {
	var n int32
	s := New()
	s.Every(50*time.Millisecond, "tick", func(_ context.Context) error {
		atomic.AddInt32(&n, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	s.Wait()
	if got := atomic.LoadInt32(&n); got < 2 {
		t.Errorf("fired %d times, want >= 2", got)
	}
}
