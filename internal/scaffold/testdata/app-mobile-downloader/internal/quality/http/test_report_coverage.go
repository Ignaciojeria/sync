package dev

import (
	authmiddleware "gitinittest5/internal/auth/middleware"
	infratest "gitinittest5/internal/shared/infrastructure/test"
	"gitinittest5/internal/shared/server"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func testReportCoverageHandler(s *server.Server) {
	server.Get(s, "/report/tests/coverage.html", testReportCoverage, server.OptionMiddleware(authmiddleware.RequireEditor()))
}

func testReportCoverage(c server.ContextNoBody) (string, error) {
	htmlReport := filepath.Join(infratest.CoverageDir, "coverage.html")
	f, err := os.Open(htmlReport)
	if err != nil {
		return "", server.HTTPError{Status: http.StatusNotFound, Detail: "report not found"}
	}
	defer f.Close()

	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.SetHeader("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.SetHeader("Pragma", "no-cache")
	c.SetHeader("Expires", "0")
	_, _ = io.Copy(c.Response(), f)
	return "", nil
}
