package dev

import (
	"net/http"

	"app-mobile-downloader/internal/quality/ui"
	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	"app-mobile-downloader/internal/shared/infrastructure/test"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(testReportPageHandler)

func testReportPageHandler(s *server.Server) {
	fuego.Get(s.Server, "/report/tests", testReportPage)
}

func testReportPage(c fuego.ContextNoBody) (any, error) {
	claims, ok := authmiddleware.JWTClaimsFromContext(c.Context())
	if !ok {
		return nil, fuego.HTTPError{Status: http.StatusUnauthorized, Detail: "unauthorized"}
	}
	email := shared.FirstStringClaim(claims, "email")
	if !access.IsAllowedEditorEmail(email) {
		return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "forbidden"}
	}

	state := test.LoadLastRunState()
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	page, err := layout.RenderPage(c, "Reporte de Tests", ui.Page(state))
	if err != nil {
		return nil, err
	}
	return nil, page.Render(c.Context(), c.Response())
}

