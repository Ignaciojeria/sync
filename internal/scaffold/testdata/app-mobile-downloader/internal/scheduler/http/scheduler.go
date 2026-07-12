package scheduler

import (
	"log/slog"
	"time"

	schedulerapp "testboi1/internal/scheduler/application"
	schedulerpostgresql "testboi1/internal/scheduler/infrastructure/postgresql"
	"testboi1/internal/shared"
	sharedpostgresql "testboi1/internal/shared/infrastructure/postgresql"

	"github.com/robfig/cron/v3"
)

func startScheduler(
	db *sharedpostgresql.Connection,
	hooks *shared.Hooks,
) error {
	if db == nil || db.DB == nil {
		slog.Warn("primary db not available, scheduler not started")
		return nil
	}

	repo := schedulerpostgresql.NewJobRepository(db)
	lock := schedulerpostgresql.NewDistributedLock(db)
	client := schedulerapp.NewInternalHTTPClient()
	runner := schedulerapp.NewJobRunner(repo, lock, client)

	c := cron.New(cron.WithSeconds())

	_, err := c.AddFunc("0 * * * * *", func() {
		jobs, err := repo.FindAll()
		if err != nil {
			slog.Error("scheduler failed to list jobs", "error", err)
			return
		}

		now := time.Now()
		for _, job := range jobs {
			if !job.Enabled {
				continue
			}
			if shouldRun(job, now) {
				go runner.Run(job)
			}
		}
	})
	if err != nil {
		return err
	}

	c.Start()

	hooks.RegisterShutdown(func() error {
		ctx := c.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(10 * time.Second):
			return nil
		}
	})

	slog.Info("scheduler started")
	return nil
}

func shouldRun(job schedulerapp.JobConfig, now time.Time) bool {
	if job.LastRunAt == nil {
		return true
	}

	schedule, err := cron.ParseStandard(job.Schedule)
	if err != nil {
		slog.Error("invalid cron schedule", "job", job.Name, "schedule", job.Schedule, "error", err)
		return false
	}

	prevRun := schedule.Next(now.Add(-time.Hour))
	if prevRun.After(now) {
		prevRun = schedule.Next(now.Add(-2 * time.Hour))
	}

	return job.LastRunAt.Before(prevRun)
}
