package dev

import (
	"net/http"
	"net/http/httptest"
	"testing"

	testreport "testboi1/internal/quality/application/test_report"
	"testboi1/internal/quality/ui"
	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func TestQualityRegister(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	runner := testreport.NewRunnerWithDeps(testreport.RunnerDeps{
		IsAllowedEditorEmail: func(string) bool { return true },
	})
	Register(s, runner)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/report/tests")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer res.Body.Close()

	// The endpoint requires editor claims; without JWTMiddleware we expect 401.
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (no claims)", res.StatusCode)
	}
}

// silence unused imports
var _ = ui.TestRunState{}
