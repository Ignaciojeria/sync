package dev

import (
	authmiddleware "fixtests1/internal/auth/middleware"
	"fixtests1/internal/shared/configuration"
	"fixtests1/internal/shared/server"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newPageTestServer(withJWTMiddleware bool) *httptest.Server {
	fs := server.NewServer()
	if withJWTMiddleware {
		server.Use(fs, authmiddleware.JWTMiddleware(nil, nil, configuration.Conf{}))
	}
	s := fs
	testReportPageHandler(s)
	return httptest.NewServer(fs.Mux)
}

func TestTestReportPage(t *testing.T) {
	t.Run("unauthorized without claims", func(t *testing.T) {
		ts := newPageTestServer(false)
		defer ts.Close()

		res, err := http.Get(ts.URL + "/report/tests")
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
		ts := newPageTestServer(true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/report/tests", nil)
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

	t.Run("success returns html page", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		ts := newPageTestServer(true)
		defer ts.Close()

		req, err := http.NewRequest(http.MethodGet, ts.URL+"/report/tests", nil)
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
	})
}
