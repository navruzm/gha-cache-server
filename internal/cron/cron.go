package cron

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	Name     string
	Interval time.Duration
	Fn       func(context.Context) error
}

type Scheduler struct {
	jobs []Job
	wg   sync.WaitGroup
	log  *slog.Logger
}

func New() *Scheduler { return &Scheduler{log: slog.Default()} }

func (s *Scheduler) WithLogger(l *slog.Logger) *Scheduler {
	s.log = l
	return s
}

func (s *Scheduler) Every(d time.Duration, name string, fn func(context.Context) error) {
	s.jobs = append(s.jobs, Job{Name: name, Interval: d, Fn: fn})
}

func (s *Scheduler) Run(ctx context.Context) {
	for _, j := range s.jobs {
		s.wg.Add(1)
		go s.runJob(ctx, j)
	}
}

func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) runJob(ctx context.Context, j Job) {
	defer s.wg.Done()
	t := time.NewTicker(j.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			func() {
				cctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
				defer cancel()
				if err := j.Fn(cctx); err != nil {
					s.log.Error("scheduled task failed", "name", j.Name, "err", err)
				}
			}()
		}
	}
}
