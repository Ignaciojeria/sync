package dev

import (
	"errors"
	authmiddleware "gitinittest5/internal/auth/middleware"
	testreport "gitinittest5/internal/quality/application/test_report"
	"gitinittest5/internal/quality/ui"
	"gitinittest5/internal/shared/configuration"
	"gitinittest5/internal/shared/server"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newRunTestServer(t *testing.T, runner *testreport.Runner, withJWTMiddleware bool) *httptest.Server {
	t.Helper()
	fs := server.NewServer()
	if withJWTMiddleware {
		server.Use(fs, authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{}))
	}
	s := fs
	testReportRunHandler(s, runner)
	return httptest.NewServer(fs.Mux)
}

func newSuccessfulRunner() *testreport.Runner {
	return testreport.NewRunnerWithDeps(testreport.RunnerDeps{
		FindProjectRoot:            func() (string, error) { return "/tmp", nil },
		EnsureCoverageDir:          func() error { return nil },
		RunTests:                   func(root, coverProfile string) ([]byte, error) { return []byte("ok"), nil },
		FilterCoverageFile:         func(input, output string) error { return nil },
		CoveragePercentFromProfile: func(root, profile string) (float64, error) { return 77.7, nil },
		GenerateHTMLReport:         func(root, filteredProfile, htmlReport string) error { return nil },
		SaveLastRunState:           func(state ui.TestRunState) error { return nil },
		IsAllowedEditorEmail:       func(email string) bool { return true },
	})
}

func TestTestReportRunHandler(t *testing.T) {
	t.Run("unauthorized when claims missing", func(t *testing.T) {
		ts := newRunTestServer(t, newSuccessfulRunner(), false)
		defer ts.Close()

		resp, err := http.Post(ts.URL+"/report/tests/run", "text/plain", nil)
		if err != nil {
			t.Fatalf("Post() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
	})

	t.Run("forbidden when email is not allowed", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		ts := newRunTestServer(t, newSuccessfulRunner(), true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/report/tests/run", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "blocked@example.com")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("forbidden when runner returns forbidden", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		runner := testreport.NewRunnerWithDeps(testreport.RunnerDeps{
			IsAllowedEditorEmail: func(email string) bool { return false },
		})
		ts := newRunTestServer(t, runner, true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/report/tests/run", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "dev@example.com")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("internal server error when runner fails", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		runner := testreport.NewRunnerWithDeps(testreport.RunnerDeps{
			IsAllowedEditorEmail: func(email string) bool { return true },
			FindProjectRoot:      func() (string, error) { return "", errors.New("boom") },
		})
		ts := newRunTestServer(t, runner, true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/report/tests/run", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "dev@example.com")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", resp.StatusCode)
		}
	})

	t.Run("success renders dashboard and result", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		runner := testreport.NewRunnerWithDeps(testreport.RunnerDeps{
			FindProjectRoot:            func() (string, error) { return "/tmp", nil },
			EnsureCoverageDir:          func() error { return nil },
			RunTests:                   func(root, coverProfile string) ([]byte, error) { return []byte("ok output"), nil },
			FilterCoverageFile:         func(input, output string) error { return nil },
			CoveragePercentFromProfile: func(root, profile string) (float64, error) { return 88.8, nil },
			GenerateHTMLReport:         func(root, filteredProfile, htmlReport string) error { return nil },
			SaveLastRunState:           func(state ui.TestRunState) error { return nil },
			IsAllowedEditorEmail:       func(email string) bool { return true },
		})
		ts := newRunTestServer(t, runner, true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodPost, ts.URL+"/report/tests/run", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "dev@example.com")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("content-type = %q", got)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if !strings.Contains(string(body), "Resumen") {
			t.Fatalf("expected html response to include summary, got %q", string(body))
		}
	})
}

func TestNewSuccessfulRunnerStateTimestampUsed(t *testing.T) {
	// Keep this tiny smoke test to ensure helper remains deterministic enough for reuse.
	r := newSuccessfulRunner()
	state, err := r.Run("dev@example.com")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !state.HasResult {
		t.Fatal("expected state with result")
	}
	if state.Timestamp.Before(time.Unix(0, 0)) {
		t.Fatal("expected valid timestamp")
	}
}
