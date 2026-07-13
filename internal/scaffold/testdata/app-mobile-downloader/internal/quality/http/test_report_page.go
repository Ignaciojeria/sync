package dev

import (
	authmiddleware "fixtests1/internal/auth/middleware"
	"fixtests1/internal/quality/ui"
	"fixtests1/internal/shared/infrastructure/test"
	"fixtests1/internal/shared/server"
	"fixtests1/internal/ui/layout"
)

func testReportPageHandler(s *server.Server) {
	server.Get(s, "/report/tests", testReportPage, server.OptionMiddleware(authmiddleware.RequireEditor()))
}

func testReportPage(c server.ContextNoBody) (any, error) {
	state := test.LoadLastRunState()
	nav := layout.FromRequest(c.Request())
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	page, err := layout.RenderPage(c, "Reporte de Tests", ui.Page(state, nav.PreviewPrefix))
	if err != nil {
		return nil, err
	}
	return nil, page.Render(c.Context(), c.Response())
}
