package scheduler

import (
	"time"
)

type JobConfig struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Schedule    string
	Endpoint    string
	LastRunAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type JobRunLog struct {
	ID           string
	JobConfigID  string
	Status       string
	Output       string
	ErrorMessage string
	StartedAt    time.Time
	FinishedAt   *time.Time
	AcquiredLock bool
}

type Repository interface {
	FindAll() ([]JobConfig, error)
	FindByID(id string) (JobConfig, error)
	FindByName(name string) (JobConfig, error)
	Create(job JobConfig) (string, error)
	Delete(id string) error
	UpdateEnabled(id string, enabled bool) error
	UpdateLastRunAt(id string, t time.Time) error
	CreateLog(log JobRunLog) (string, error)
	UpdateLog(log JobRunLog) error
	FindLogs(jobConfigID string, limit int) ([]JobRunLog, error)
}

type DistributedLock interface {
	Acquire(jobName string) (bool, func() error)
}

type HTTPClient interface {
	Post(endpoint string) error
}
