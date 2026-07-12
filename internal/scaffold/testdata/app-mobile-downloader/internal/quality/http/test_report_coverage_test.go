package dev

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authmiddleware "testboi1/internal/auth/middleware"
	"testboi1/internal/shared/configuration"
	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func newCoverageTestServer(withJWTMiddleware bool) *httptest.Server {
	fs := fuego.NewServer()
	if withJWTMiddleware {
		fuego.Use(fs, authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{}))
	}
	s := &server.Server{Server: fs}
	testReportCoverageHandler(s)
	return httptest.NewServer(fs.Mux)
}

func TestTestReportCoverage(t *testing.T) {
	t.Run("unauthorized without claims", func(t *testing.T) {
		ts := newCoverageTestServer(false)
		defer ts.Close()

		res, err := http.Get(ts.URL + "/report/tests/coverage.html")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", res.StatusCode)
		}
	})

	t.Run("forbidden for non-allowed email", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		ts := newCoverageTestServer(true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/report/tests/coverage.html", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "blocked@example.com")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.StatusCode)
		}
	})

	t.Run("not found when report file missing", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")

		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd() error = %v", err)
		}
		tmp := t.TempDir()
		if err := os.Chdir(tmp); err != nil {
			t.Fatalf("Chdir() error = %v", err)
		}
		defer func() { _ = os.Chdir(wd) }()

		ts := newCoverageTestServer(true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/report/tests/coverage.html", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "dev@example.com")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
	})

	t.Run("success serves html and cache headers", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")

		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd() error = %v", err)
		}
		tmp := t.TempDir()
		if err := os.Chdir(tmp); err != nil {
			t.Fatalf("Chdir() error = %v", err)
		}
		defer func() { _ = os.Chdir(wd) }()

		reportDir := filepath.Join("tmp", "coverage")
		if err := os.MkdirAll(reportDir, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		report := "<html><body>coverage ok</body></html>"
		if err := os.WriteFile(filepath.Join(reportDir, "coverage.html"), []byte(report), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		ts := newCoverageTestServer(true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/report/tests/coverage.html", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("X-Dev-Email", "dev@example.com")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		if got := res.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
			t.Fatalf("content-type = %q", got)
		}
		if got := res.Header.Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, max-age=0" {
			t.Fatalf("cache-control = %q", got)
		}
		if got := res.Header.Get("Pragma"); got != "no-cache" {
			t.Fatalf("pragma = %q", got)
		}
		if got := res.Header.Get("Expires"); got != "0" {
			t.Fatalf("expires = %q", got)
		}
	})
}
