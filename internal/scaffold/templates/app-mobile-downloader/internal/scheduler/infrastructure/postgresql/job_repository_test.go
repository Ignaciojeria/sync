package postgresql

import (
	"database/sql"
	"hash/fnv"
	"regexp"
	"testing"
	"time"

	schedulerapp "app-mobile-downloader/internal/scheduler/application"
	sharedpostgresql "app-mobile-downloader/internal/shared/infrastructure/postgresql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func newJobRepoMock(t *testing.T) (*JobRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}
	return NewJobRepository(conn), mock
}

func newDistributedLockMock(t *testing.T) (*DistributedLock, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}
	return NewDistributedLock(conn), mock
}

func TestNewJobRepository(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}
	r := NewJobRepository(conn)
	if r == nil || r.db != conn {
		t.Fatal("expected repository to keep provided connection")
	}
}

func TestFindAll(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lastRun := now.Add(-time.Hour)

	rows := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("a", "job-a", "do a", true, "* * * * *", "/a", lastRun, now, now).
		AddRow("b", "job-b", "do b", false, "0 * * * *", "/b", nil, now, now)

	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at").
		WillReturnRows(rows)

	got, err := repo.FindAll()
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != "a" || got[0].Name != "job-a" || !got[0].Enabled {
		t.Fatalf("first job = %+v", got[0])
	}
	if got[0].LastRunAt == nil || !got[0].LastRunAt.Equal(lastRun) {
		t.Fatalf("LastRunAt = %v, want %v", got[0].LastRunAt, lastRun)
	}
	if got[1].LastRunAt != nil {
		t.Fatalf("LastRunAt(b) = %v, want nil", got[1].LastRunAt)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestFindAllQueryError(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	mock.ExpectQuery("SELECT id, name").WillReturnError(sql.ErrNoRows)
	if _, err := repo.FindAll(); err == nil {
		t.Fatal("expected error")
	}
}

func TestFindByID(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("j-1", "job-1", "", true, "* * * * *", "/x", nil, now, now)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = $1")).
		WithArgs("j-1").
		WillReturnRows(rows)

	got, err := repo.FindByID("j-1")
	if err != nil {
		t.Fatalf("FindByID error = %v", err)
	}
	if got.ID != "j-1" || got.Name != "job-1" {
		t.Fatalf("got = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}

	mock.ExpectQuery("SELECT id, name").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)
	if _, err := repo.FindByID("missing"); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestFindByName(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("j-1", "job-1", "", true, "* * * * *", "/x", now, now, now)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE name = $1")).
		WithArgs("job-1").
		WillReturnRows(rows)
	got, err := repo.FindByName("job-1")
	if err != nil {
		t.Fatalf("FindByName error = %v", err)
	}
	if got.Name != "job-1" {
		t.Fatalf("got = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}

	mock.ExpectQuery("SELECT id, name").
		WithArgs("missing-name").
		WillReturnError(sql.ErrNoRows)
	if _, err := repo.FindByName("missing-name"); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCreate(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	job := schedulerapp.JobConfig{
		Name:        "name",
		Description: "desc",
		Enabled:     false,
		Schedule:    "* * * * *",
		Endpoint:    "/x",
	}

	mock.ExpectQuery("INSERT INTO job_configs").
		WithArgs("name", "desc", false, "* * * * *", "/x").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("j-new"))

	id, err := repo.Create(job)
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if id != "j-new" {
		t.Fatalf("id = %q", id)
	}

	mock.ExpectQuery("INSERT INTO job_configs").
		WillReturnError(sql.ErrConnDone)
	if _, err := repo.Create(job); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestDelete(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	mock.ExpectExec("DELETE FROM job_configs WHERE id").
		WithArgs("j-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.Delete("j-1"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}

	mock.ExpectExec("DELETE FROM job_configs WHERE id").
		WillReturnError(sql.ErrNoRows)
	if err := repo.Delete("j-missing"); err == nil {
		t.Fatal("expected delete error")
	}
}

func TestUpdateEnabled(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	mock.ExpectExec("UPDATE job_configs SET enabled").
		WithArgs(true, "j-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateEnabled("j-1", true); err != nil {
		t.Fatalf("UpdateEnabled error = %v", err)
	}

	mock.ExpectExec("UPDATE job_configs SET enabled").
		WillReturnError(sql.ErrNoRows)
	if err := repo.UpdateEnabled("j-1", true); err == nil {
		t.Fatal("expected update error")
	}
}

func TestUpdateLastRunAt(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	now := time.Now().UTC()
	mock.ExpectExec("UPDATE job_configs SET last_run_at").
		WithArgs(now, "j-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateLastRunAt("j-1", now); err != nil {
		t.Fatalf("UpdateLastRunAt error = %v", err)
	}

	mock.ExpectExec("UPDATE job_configs SET last_run_at").
		WillReturnError(sql.ErrConnDone)
	if err := repo.UpdateLastRunAt("j-1", now); err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateLog(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	log := schedulerapp.JobRunLog{JobConfigID: "j-1", Status: "running", StartedAt: time.Now(), AcquiredLock: true}

	mock.ExpectQuery("INSERT INTO job_run_logs").
		WithArgs("j-1", "running", log.StartedAt, true).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("log-1"))

	id, err := repo.CreateLog(log)
	if err != nil {
		t.Fatalf("CreateLog error = %v", err)
	}
	if id != "log-1" {
		t.Fatalf("id = %q", id)
	}

	mock.ExpectQuery("INSERT INTO job_run_logs").
		WillReturnError(sql.ErrConnDone)
	if _, err := repo.CreateLog(log); err == nil {
		t.Fatal("expected insert error")
	}
}

func TestUpdateLog(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	finished := time.Now()
	log := schedulerapp.JobRunLog{ID: "log-1", Status: "success", FinishedAt: &finished}

	mock.ExpectExec("UPDATE job_run_logs SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.UpdateLog(log); err != nil {
		t.Fatalf("UpdateLog error = %v", err)
	}

	mock.ExpectExec("UPDATE job_run_logs SET status").
		WillReturnError(sql.ErrConnDone)
	if err := repo.UpdateLog(log); err == nil {
		t.Fatal("expected update error")
	}
}

func TestFindLogs(t *testing.T) {
	repo, mock := newJobRepoMock(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "job_config_id", "status", "output", "error_message", "started_at", "finished_at", "acquired_lock"}).
		AddRow("l1", "j-1", "success", "ok", "", now, sql.NullTime{Valid: true, Time: now.Add(5 * time.Second)}, true).
		AddRow("l2", "j-1", "failed", "boom", "runtime", now, sql.NullTime{}, false)

	mock.ExpectQuery("SELECT id, job_config_id, status, output, error_message, started_at, finished_at, acquired_lock").
		WithArgs("j-1", 50).
		WillReturnRows(rows)

	logs, err := repo.FindLogs("j-1", 50)
	if err != nil {
		t.Fatalf("FindLogs error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("len = %d", len(logs))
	}
	if logs[0].FinishedAt == nil {
		t.Fatal("expected finished_at for first log")
	}
	if logs[1].FinishedAt != nil {
		t.Fatal("expected nil finished_at for second log")
	}

	mock.ExpectQuery("SELECT id, job_config_id").
		WillReturnError(sql.ErrConnDone)
	if _, err := repo.FindLogs("j-1", 50); err == nil {
		t.Fatal("expected find error")
	}
}

func TestDistributedLockAcquireSuccess(t *testing.T) {
	lock, mock := newDistributedLockMock(t)

	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(true))

	acquired, release := lock.Acquire("job-success")
	if !acquired {
		t.Fatal("expected acquired=true")
	}

	mock.ExpectExec("SELECT pg_advisory_unlock").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := release(); err != nil {
		t.Fatalf("release error = %v", err)
	}
}

func TestDistributedLockAcquireFailure(t *testing.T) {
	lock, mock := newDistributedLockMock(t)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(false))

	acquired, release := lock.Acquire("job-fail")
	if acquired {
		t.Fatal("expected acquired=false")
	}
	if err := release(); err != nil {
		t.Fatalf("release error = %v", err)
	}
}

func TestDistributedLockAcquireQueryError(t *testing.T) {
	lock, mock := newDistributedLockMock(t)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WillReturnError(sql.ErrConnDone)

	acquired, release := lock.Acquire("job-error")
	if acquired {
		t.Fatal("expected acquired=false on query error")
	}
	if err := release(); err != nil {
		t.Fatalf("release error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet: %v", err)
	}
}

func TestDistributedLockReleaseError(t *testing.T) {
	lock, mock := newDistributedLockMock(t)
	mock.ExpectQuery("SELECT pg_try_advisory_lock").
		WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(true))

	acquired, release := lock.Acquire("job-release")
	if !acquired {
		t.Fatal("expected acquired=true")
	}
	mock.ExpectExec("SELECT pg_advisory_unlock").
		WillReturnError(sql.ErrConnDone)

	if err := release(); err == nil {
		t.Fatal("expected release error")
	}
}

func TestHashJobNameDeterministic(t *testing.T) {
	if hashJobName("job") != hashJobName("job") {
		t.Fatal("expected identical hashes for same input")
	}
	if hashJobName("job") == hashJobName("job-other") {
		t.Fatal("expected different hashes for different inputs")
	}
}

// Sanity check: hashJobName matches fnv.New64a of the same input — verifies
// the documented behaviour without exposing the helper.
func TestHashJobNameFNV64a(t *testing.T) {
	h := fnv.New64a()
	_, _ = h.Write([]byte("hello"))
	if hashJobName("hello") != int64(h.Sum64()) {
		t.Fatal("expected hashJobName to match fnv-64a of input")
	}
}
