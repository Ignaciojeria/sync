package postgresql

import (
	"database/sql"
	"hash/fnv"
	"time"

	schedulerapp "fixtests1/internal/scheduler/application"
	sharedpostgresql "fixtests1/internal/shared/infrastructure/postgresql"
)

type JobRepository struct {
	db *sharedpostgresql.Connection
}

func NewJobRepository(db *sharedpostgresql.Connection) *JobRepository {
	return &JobRepository{db: db}
}

func (r *JobRepository) FindAll() ([]schedulerapp.JobConfig, error) {
	query := "SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs ORDER BY name"
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []schedulerapp.JobConfig
	for rows.Next() {
		var c schedulerapp.JobConfig
		var lastRunAt sql.NullTime
		err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Schedule, &c.Endpoint, &lastRunAt, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if lastRunAt.Valid {
			c.LastRunAt = &lastRunAt.Time
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (r *JobRepository) FindByID(id string) (schedulerapp.JobConfig, error) {
	var c schedulerapp.JobConfig
	var lastRunAt sql.NullTime
	query := "SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = $1"
	err := r.db.QueryRowx(query, id).Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Schedule, &c.Endpoint, &lastRunAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	if lastRunAt.Valid {
		c.LastRunAt = &lastRunAt.Time
	}
	return c, nil
}

func (r *JobRepository) FindByName(name string) (schedulerapp.JobConfig, error) {
	var c schedulerapp.JobConfig
	var lastRunAt sql.NullTime
	query := "SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE name = $1"
	err := r.db.QueryRowx(query, name).Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Schedule, &c.Endpoint, &lastRunAt, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, err
	}
	if lastRunAt.Valid {
		c.LastRunAt = &lastRunAt.Time
	}
	return c, nil
}

func (r *JobRepository) Create(job schedulerapp.JobConfig) (string, error) {
	var id string
	query := "INSERT INTO job_configs (name, description, enabled, schedule, endpoint, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id"
	err := r.db.QueryRowx(query, job.Name, job.Description, job.Enabled, job.Schedule, job.Endpoint).Scan(&id)
	return id, err
}

func (r *JobRepository) Delete(id string) error {
	_, err := r.db.Exec("DELETE FROM job_configs WHERE id = $1", id)
	return err
}

func (r *JobRepository) UpdateEnabled(id string, enabled bool) error {
	_, err := r.db.Exec("UPDATE job_configs SET enabled = $1, updated_at = NOW() WHERE id = $2", enabled, id)
	return err
}

func (r *JobRepository) UpdateLastRunAt(id string, t time.Time) error {
	_, err := r.db.Exec("UPDATE job_configs SET last_run_at = $1, updated_at = NOW() WHERE id = $2", t, id)
	return err
}

func (r *JobRepository) CreateLog(log schedulerapp.JobRunLog) (string, error) {
	var id string
	query := "INSERT INTO job_run_logs (job_config_id, status, started_at, acquired_lock) VALUES ($1, $2, $3, $4) RETURNING id"
	err := r.db.QueryRowx(query, log.JobConfigID, log.Status, log.StartedAt, log.AcquiredLock).Scan(&id)
	return id, err
}

func (r *JobRepository) UpdateLog(log schedulerapp.JobRunLog) error {
	_, err := r.db.Exec(
		"UPDATE job_run_logs SET status = $1, output = $2, error_message = $3, finished_at = $4 WHERE id = $5",
		log.Status, log.Output, log.ErrorMessage, log.FinishedAt, log.ID,
	)
	return err
}

func (r *JobRepository) FindLogs(jobConfigID string, limit int) ([]schedulerapp.JobRunLog, error) {
	query := "SELECT id, job_config_id, status, output, error_message, started_at, finished_at, acquired_lock FROM job_run_logs WHERE job_config_id = $1 ORDER BY started_at DESC LIMIT $2"
	rows, err := r.db.Query(query, jobConfigID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []schedulerapp.JobRunLog
	for rows.Next() {
		var l schedulerapp.JobRunLog
		var finishedAt sql.NullTime
		err := rows.Scan(&l.ID, &l.JobConfigID, &l.Status, &l.Output, &l.ErrorMessage, &l.StartedAt, &finishedAt, &l.AcquiredLock)
		if err != nil {
			return nil, err
		}
		if finishedAt.Valid {
			l.FinishedAt = &finishedAt.Time
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

type DistributedLock struct {
	db *sharedpostgresql.Connection
}

func NewDistributedLock(db *sharedpostgresql.Connection) *DistributedLock {
	return &DistributedLock{db: db}
}

func (l *DistributedLock) Acquire(jobName string) (bool, func() error) {
	key := hashJobName(jobName)
	var acquired bool
	err := l.db.QueryRow("SELECT pg_try_advisory_lock($1)", key).Scan(&acquired)
	if err != nil || !acquired {
		return false, func() error { return nil }
	}
	return true, func() error {
		_, err := l.db.Exec("SELECT pg_advisory_unlock($1)", key)
		return err
	}
}

func hashJobName(name string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return int64(h.Sum64())
}
