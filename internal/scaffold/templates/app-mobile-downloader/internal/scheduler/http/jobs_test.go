package scheduler

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/configuration"
	sharedpostgresql "app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-fuego/fuego"
	"github.com/jmoiron/sqlx"
)

// newSchedulerMockBoot creates a sqlmock-backed connection and a fuego server
// with the JWT middleware in AUTH_DISABLED mode (test-side bypass). The returned
// *httptest.Server is wired to the same sqlx.DB that the mock expectations are
// attached to.
func newSchedulerMockBoot(t *testing.T) (*httptest.Server, sqlmock.Sqlmock) {
	t.Helper()
	t.Setenv("AUTH_DISABLED", "true")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	fs := fuego.NewServer()
	fuego.Use(fs, authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{}))
	s := &server.Server{Server: fs}
	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}
	jobConfigPageHandler(s, conn)
	jobConfigAPIHandler(s, conn)
	ts := httptest.NewServer(fs.Mux)
	t.Cleanup(ts.Close)
	return ts, mock
}

func editorGet(url string) *http.Request {
	return newEditorRequest(http.MethodGet, url, nil)
}

func editorPost(url, contentType string, body io.Reader) *http.Request {
	req, _ := http.NewRequest(http.MethodPost, url, body)
	req.Header.Set("X-Dev-Email", "ignaajeriag@falabella.cl")
	req.Header.Set("X-Dev-Sub", "test-user")
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	return req
}

func editorDelete(url string) *http.Request {
	return newEditorRequest(http.MethodDelete, url, nil)
}

func newEditorRequest(method, url string, body io.Reader) *http.Request {
	req, _ := http.NewRequest(method, url, body)
	req.Header.Set("X-Dev-Email", "ignaajeriag@falabella.cl")
	req.Header.Set("X-Dev-Sub", "test-user")
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return req
}

func doEditor(t *testing.T, ts *httptest.Server, req *http.Request) *http.Response {
	t.Helper()
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	return res
}

func TestListJobsPageReturnsHtml(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("a", "job-a", "", true, "* * * * *", "/x", nil, now, now)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at").
		WillReturnRows(rows)

	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestListJobsPageErrorReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("SELECT id, name").WillReturnError(sql.ErrConnDone)

	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestListJobsPageErrorRowsScanFailsReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	badRows := sqlmock.NewRows([]string{"only-one-column"}).AddRow("a")
	mock.ExpectQuery("SELECT id, name, description").WillReturnRows(badRows)

	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when rows scan fails", res.StatusCode)
	}
}

func TestNewJobFormReturnsForm(t *testing.T) {
	ts, _ := newSchedulerMockBoot(t)
	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs/new"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestCancelJobFormReturnsFragment(t *testing.T) {
	ts, _ := newSchedulerMockBoot(t)
	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs/cancel"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestCreateJobSuccess(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()

	mock.ExpectQuery("INSERT INTO job_configs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-1"))
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
			AddRow("new-1", "nightly", "fancy", false, "0 * * * *", "/x", nil, now, now))

	form := url.Values{}
	form.Set("name", "nightly")
	form.Set("description", "fancy")
	form.Set("schedule", "0 * * * *")
	form.Set("endpoint", "/x")

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestCreateJobMissingFieldsReturns400(t *testing.T) {
	ts, _ := newSchedulerMockBoot(t)

	form := url.Values{}
	form.Set("name", "only-name")

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestCreateJobRepoErrorReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("INSERT INTO job_configs").
		WillReturnError(sql.ErrConnDone)

	form := url.Values{}
	form.Set("name", "x")
	form.Set("schedule", "* * * * *")
	form.Set("endpoint", "/x")

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestCreateJobPostLookupErrorReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("INSERT INTO job_configs").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("new-2"))
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnError(sql.ErrConnDone)

	form := url.Values{}
	form.Set("name", "y")
	form.Set("schedule", "* * * * *")
	form.Set("endpoint", "/y")

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs", "application/x-www-form-urlencoded", strings.NewReader(form.Encode())))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestDeleteJobSuccess(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectExec("DELETE FROM job_configs WHERE id").
		WillReturnResult(sqlmock.NewResult(0, 1))

	res := doEditor(t, ts, editorDelete(ts.URL+"/scheduler/jobs/j-1"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestDeleteJobRepoErrorReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectExec("DELETE FROM job_configs WHERE id").
		WillReturnError(sql.ErrConnDone)

	res := doEditor(t, ts, editorDelete(ts.URL+"/scheduler/jobs/j-err"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestToggleJobSuccess(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()
	rows1 := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("j-1", "job-1", "", false, "* * * * *", "/x", nil, now, now)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnRows(rows1)
	mock.ExpectExec("UPDATE job_configs SET enabled").
		WillReturnResult(sqlmock.NewResult(0, 1))
	rows2 := sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
		AddRow("j-1", "job-1", "", true, "* * * * *", "/x", nil, now, now)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnRows(rows2)

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs/j-1/toggle", "application/x-www-form-urlencoded", nil))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestToggleJobMissingJobReturns404(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnError(sql.ErrNoRows)

	res := doEditor(t, ts, editorPost(ts.URL+"/scheduler/jobs/missing/toggle", "application/x-www-form-urlencoded", nil))
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

func TestAPIListJobs(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
			AddRow("a", "j-a", "", true, "* * * * *", "/a", nil, now, now))

	res := doEditor(t, ts, editorGet(ts.URL+"/api/internal/jobs"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestAPIListJobsErrorReturns500(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at").
		WillReturnError(sql.ErrConnDone)

	res := doEditor(t, ts, editorGet(ts.URL+"/api/internal/jobs"))
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", res.StatusCode)
	}
}

func TestAPITriggerJobEnabled(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
			AddRow("j-1", "job-1", "", true, "* * * * *", "/x", nil, now, now))

	res := doEditor(t, ts, editorPost(ts.URL+"/api/internal/jobs/j-1/run", "application/json", nil))
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestAPITriggerJobDisabledReturns400(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "enabled", "schedule", "endpoint", "last_run_at", "created_at", "updated_at"}).
			AddRow("j-1", "job-1", "", false, "* * * * *", "/x", nil, now, now))

	res := doEditor(t, ts, editorPost(ts.URL+"/api/internal/jobs/j-1/run", "application/json", nil))
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestAPITriggerJobMissingReturns404(t *testing.T) {
	ts, mock := newSchedulerMockBoot(t)
	mock.ExpectQuery("SELECT id, name, description, enabled, schedule, endpoint, last_run_at, created_at, updated_at FROM job_configs WHERE id = ?").
		WillReturnError(sql.ErrNoRows)

	res := doEditor(t, ts, editorPost(ts.URL+"/api/internal/jobs/missing/run", "application/json", nil))
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", res.StatusCode)
	}
}

// TestRegister wires all scheduler routes and starts the cron loop in a
// sqlmock-backed environment. The cron tick("0 * * * * *") would only fire at
// the next minute boundary, which this test does not reach.
//
//nolint:paralleltest // registers a long-running cron schedule that must be torn down
func TestRegister(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "true")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	hooks := &shared.Hooks{}
	fs := fuego.NewServer()
	fuego.Use(fs, authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{}))
	s := &server.Server{Server: fs}
	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}

	// The cron tick fires at second :00 of every minute; in practice tests run
	// far below that boundary. We still expect any tick to issue ONLY FindAll,
	// which we anticipate by registering none and tolerating the unknown query.
	mock.MatchExpectationsInOrder(false)

	if err := Register(s, conn, hooks); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	// Confirm schedule handler is wired.
	res := doEditor(t, ts, editorGet(ts.URL+"/scheduler/jobs"))
	res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 200 or 500", res.StatusCode)
	}

	// Tear down the cron loop so it does not leak past the test.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := hooks.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
