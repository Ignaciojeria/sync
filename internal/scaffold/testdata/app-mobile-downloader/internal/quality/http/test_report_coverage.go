package dev

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	authmiddleware "scaffoldxd1/internal/auth/middleware"
	"scaffoldxd1/internal/shared/server"
	infratest "scaffoldxd1/internal/shared/infrastructure/test"

	"github.com/go-fuego/fuego"
)

func testReportCoverageHandler(s *server.Server) {
	fuego.Get(s.Server, "/report/tests/coverage.html", testReportCoverage, fuego.OptionMiddleware(authmiddleware.RequireEditor()))
}

func testReportCoverage(c fuego.ContextNoBody) (string, error) {
	htmlReport := filepath.Join(infratest.CoverageDir, "coverage.html")
	f, err := os.Open(htmlReport)
	if err != nil {
		return "", fuego.HTTPError{Status: http.StatusNotFound, Detail: "report not found"}
	}
	defer f.Close()

	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.SetHeader("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.SetHeader("Pragma", "no-cache")
	c.SetHeader("Expires", "0")
	_, _ = io.Copy(c.Response(), f)
	return "", nil
}

