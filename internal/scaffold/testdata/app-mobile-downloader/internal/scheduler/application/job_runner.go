package scheduler

import (
	"fmt"
	"log/slog"
	"time"
)

type JobRunner struct {
	repo   Repository
	lock   DistributedLock
	client HTTPClient
}

func NewJobRunner(repo Repository, lock DistributedLock, client HTTPClient) *JobRunner {
	return &JobRunner{
		repo:   repo,
		lock:   lock,
		client: client,
	}
}

func (r *JobRunner) Run(job JobConfig) {
	startedAt := time.Now()

	acquired, release := r.lock.Acquire(job.Name)
	if !acquired {
		slog.Info("job lock not acquired, skipping execution", "job", job.Name)
		return
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("failed to release lock", "job", job.Name, "error", err)
		}
	}()

	if err := r.repo.UpdateLastRunAt(job.ID, startedAt); err != nil {
		slog.Error("failed to update last_run_at", "job", job.Name, "error", err)
	}

	log := JobRunLog{
		JobConfigID:  job.ID,
		StartedAt:    startedAt,
		AcquiredLock: true,
		Status:       "running",
	}
	logID, err := r.repo.CreateLog(log)
	if err != nil {
		slog.Error("failed to create job log", "job", job.Name, "error", err)
	} else {
		log.ID = logID
	}

	var errMsg string
	if err := r.client.Post(job.Endpoint); err != nil {
		errMsg = err.Error()
		log.Status = "failed"
		log.ErrorMessage = errMsg
		slog.Error("job execution failed", "job", job.Name, "error", err)
	} else {
		log.Status = "success"
		slog.Info("job executed successfully", "job", job.Name)
	}

	finishedAt := time.Now()
	log.FinishedAt = &finishedAt
	if err := r.repo.UpdateLog(log); err != nil {
		slog.Error("failed to update job log", "job", job.Name, "error", err)
	}
}

func (r *JobRunner) RunByName(name string) error {
	job, err := r.repo.FindByName(name)
	if err != nil {
		return fmt.Errorf("job not found: %w", err)
	}
	if !job.Enabled {
		return fmt.Errorf("job %s is disabled", name)
	}
	r.Run(job)
	return nil
}

func (r *JobRunner) RunAll() {
	jobs, err := r.repo.FindAll()
	if err != nil {
		slog.Error("failed to list jobs", "error", err)
		return
	}
	for _, job := range jobs {
		if !job.Enabled {
			continue
		}
		r.Run(job)
	}
}
