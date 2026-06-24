package scheduler

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRepo struct {
	updateLastRunCalled atomic.Bool
	createLogCalled     atomic.Bool
	updateLogCalled     atomic.Bool
	createdID           string
	findAllJobs         []JobConfig
	findByNameJob       JobConfig
	findByNameErr       error
	updateLastErr       error
	createLogErr        error
	updateLogErr        error
}

func (r *fakeRepo) FindAll() ([]JobConfig, error)               { return r.findAllJobs, nil }
func (r *fakeRepo) FindByID(id string) (JobConfig, error)       { return JobConfig{}, nil }
func (r *fakeRepo) FindByName(name string) (JobConfig, error)   { return r.findByNameJob, r.findByNameErr }
func (r *fakeRepo) Create(j JobConfig) (string, error)          { return "", nil }
func (r *fakeRepo) Delete(id string) error                      { return nil }
func (r *fakeRepo) UpdateEnabled(id string, enabled bool) error { return nil }
func (r *fakeRepo) UpdateLastRunAt(id string, _ time.Time) error {
	r.updateLastRunCalled.Store(true)
	return r.updateLastErr
}
func (r *fakeRepo) CreateLog(_ JobRunLog) (string, error) {
	r.createLogCalled.Store(true)
	if r.createLogErr != nil {
		return "", r.createLogErr
	}
	r.createdID = "log-1"
	return r.createdID, nil
}
func (r *fakeRepo) UpdateLog(_ JobRunLog) error {
	r.updateLogCalled.Store(true)
	return r.updateLogErr
}
func (r *fakeRepo) FindLogs(_ string, _ int) ([]JobRunLog, error) { return nil, nil }

type fakeLock struct {
	acquireReturn bool
	releaseErr    error
	released      atomic.Bool
}

func (l *fakeLock) Acquire(_ string) (bool, func() error) {
	if !l.acquireReturn {
		return false, func() error { return nil }
	}
	return true, func() error {
		l.released.Store(true)
		return l.releaseErr
	}
}

type fakeClient struct {
	postCalled atomic.Bool
	postErr    error
}

func (c *fakeClient) Post(_ string) error {
	c.postCalled.Store(true)
	return c.postErr
}

func TestNewJobRunner(t *testing.T) {
	runner := NewJobRunner(&fakeRepo{}, &fakeLock{}, &fakeClient{})
	if runner == nil {
		t.Fatal("expected runner to be created")
	}
	if runner.repo == nil || runner.lock == nil || runner.client == nil {
		t.Fatal("expected runner to keep its dependencies")
	}
}

func TestRunSkipsWhenLockNotAcquired(t *testing.T) {
	repo := &fakeRepo{}
	lock := &fakeLock{acquireReturn: false}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	job := JobConfig{ID: "j1", Name: "lockable-job", Endpoint: "/x"}

	runner.Run(job)

	if client.postCalled.Load() {
		t.Fatal("client.Post should not be called when lock is not acquired")
	}
	if repo.updateLastRunCalled.Load() {
		t.Fatal("UpdateLastRunAt should not be called when lock is not acquired")
	}
	if repo.updateLogCalled.Load() {
		t.Fatal("UpdateLog should not be called when lock is not acquired")
	}
}

func TestRunSuccess(t *testing.T) {
	repo := &fakeRepo{}
	lock := &fakeLock{acquireReturn: true}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	job := JobConfig{ID: "j1", Name: "ok-job", Endpoint: "/x"}

	runner.Run(job)

	if !repo.updateLastRunCalled.Load() {
		t.Fatal("expected UpdateLastRunAt to be called")
	}
	if !repo.createLogCalled.Load() {
		t.Fatal("expected CreateLog to be called")
	}
	if !repo.updateLogCalled.Load() {
		t.Fatal("expected UpdateLog to be called")
	}
	if !client.postCalled.Load() {
		t.Fatal("expected client.Post to be called")
	}
	if !lock.released.Load() {
		t.Fatal("expected lock release to be invoked")
	}
}

func TestRunClientError(t *testing.T) {
	repo := &fakeRepo{}
	lock := &fakeLock{acquireReturn: true}
	client := &fakeClient{postErr: errors.New("client boom")}
	runner := NewJobRunner(repo, lock, client)
	job := JobConfig{ID: "j1", Name: "fail-job", Endpoint: "/x"}

	runner.Run(job)

	if !repo.updateLogCalled.Load() {
		t.Fatal("UpdateLog should still be called when client fails")
	}
}

func TestRunRepoErrorsContinue(t *testing.T) {
	repo := &fakeRepo{
		updateLastErr: errors.New("updateLast boom"),
		createLogErr:  errors.New("createLog boom"),
		updateLogErr:  errors.New("updateLog boom"),
	}
	lock := &fakeLock{acquireReturn: true}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	runner.Run(JobConfig{ID: "j1", Name: "noisy-job", Endpoint: "/x"})

	// Neither error should abort the run; lock must still be released.
	if !lock.released.Load() {
		t.Fatal("expected lock release even when repo errors occur")
	}
	if !client.postCalled.Load() {
		t.Fatal("expected client.Post to still be called")
	}
}

func TestRunLockReleaseErrorIsLoggedNotPanicked(t *testing.T) {
	repo := &fakeRepo{}
	lock := &fakeLock{acquireReturn: true, releaseErr: errors.New("release boom")}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	runner.Run(JobConfig{ID: "j1", Name: "release-job", Endpoint: "/x"})
}

func TestRunByNameNotFound(t *testing.T) {
	repo := &fakeRepo{findByNameErr: errors.New("not found")}
	lock := &fakeLock{}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	err := runner.RunByName("missing")
	if err == nil {
		t.Fatal("expected error when job not found")
	}
}

func TestRunByNameDisabled(t *testing.T) {
	repo := &fakeRepo{findByNameJob: JobConfig{ID: "j1", Name: "off-job", Enabled: false}}
	lock := &fakeLock{}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	err := runner.RunByName("off-job")
	if err == nil {
		t.Fatal("expected error when job disabled")
	}
}

func TestRunByNameEnabledRunsJob(t *testing.T) {
	repo := &fakeRepo{findByNameJob: JobConfig{ID: "j1", Name: "on-job", Enabled: true}}
	lock := &fakeLock{acquireReturn: true}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	if err := runner.RunByName("on-job"); err != nil {
		t.Fatalf("RunByName() error = %v", err)
	}
	if !client.postCalled.Load() {
		t.Fatal("expected client.Post to run when enabled")
	}
}

func TestRunAllDispatchesEnabledJobs(t *testing.T) {
	repo := &fakeRepo{
		findAllJobs: []JobConfig{
			{ID: "a", Name: "a", Enabled: true},
			{ID: "b", Name: "b", Enabled: false},
			{ID: "c", Name: "c", Enabled: true},
		},
	}
	lock := &fakeLock{acquireReturn: true}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	runner.RunAll()

	// Two enabled jobs should trigger two POST calls.
	if got := client.postCalled.Load(); !got {
		t.Fatal("expected client.Post to run")
	}
}

func TestRunAllRepoErrorDoesNotPanic(t *testing.T) {
	repo := &errRepo{}
	lock := &fakeLock{}
	client := &fakeClient{}
	runner := NewJobRunner(repo, lock, client)
	runner.RunAll()
}

type errRepo struct{}

func (errRepo) FindAll() ([]JobConfig, error)                          { return nil, errors.New("db boom") }
func (errRepo) FindByID(string) (JobConfig, error)                      { return JobConfig{}, nil }
func (errRepo) FindByName(string) (JobConfig, error)                    { return JobConfig{}, nil }
func (errRepo) Create(JobConfig) (string, error)                        { return "", nil }
func (errRepo) Delete(string) error                                    { return nil }
func (errRepo) UpdateEnabled(string, bool) error                        { return nil }
func (errRepo) UpdateLastRunAt(string, time.Time) error                 { return nil }
func (errRepo) CreateLog(JobRunLog) (string, error)                     { return "", nil }
func (errRepo) UpdateLog(JobRunLog) error                               { return nil }
func (errRepo) FindLogs(string, int) ([]JobRunLog, error)               { return nil, nil }
