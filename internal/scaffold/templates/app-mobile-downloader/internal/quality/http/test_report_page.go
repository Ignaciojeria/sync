package dev

import (
	"app-mobile-downloader/internal/quality/ui"
	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared/infrastructure/test"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func testReportPageHandler(s *server.Server) {
	fuego.Get(s.Server, "/report/tests", testReportPage, fuego.OptionMiddleware(authmiddleware.RequireEditor()))
}

func testReportPage(c fuego.ContextNoBody) (any, error) {
	state := test.LoadLastRunState()
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	page, err := layout.RenderPage(c, "Reporte de Tests", ui.Page(state))
	if err != nil {
		return nil, err
	}
	return nil, page.Render(c.Context(), c.Response())
}

