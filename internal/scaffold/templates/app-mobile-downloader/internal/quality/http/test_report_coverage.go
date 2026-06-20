package dev

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	"app-mobile-downloader/internal/shared/server"
	infratest "app-mobile-downloader/internal/shared/infrastructure/test"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(testReportCoverageHandler)

func testReportCoverageHandler(s *server.Server) {
	fuego.Get(s.Server, "/report/tests/coverage.html", testReportCoverage)
}

func testReportCoverage(c fuego.ContextNoBody) (string, error) {
	claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
	if !ok {
		return "", fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
	}
	email := shared.FirstStringClaim(claims, "email")
	if !access.IsAllowedEditorEmail(email) {
		return "", fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
	}

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

