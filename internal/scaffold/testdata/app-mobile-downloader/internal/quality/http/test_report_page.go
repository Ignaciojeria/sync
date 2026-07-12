package dev

import (
	authmiddleware "testboi1/internal/auth/middleware"
	"testboi1/internal/quality/ui"
	"testboi1/internal/shared/infrastructure/test"
	"testboi1/internal/shared/server"
	"testboi1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func testReportPageHandler(s *server.Server) {
	fuego.Get(s.Server, "/report/tests", testReportPage, fuego.OptionMiddleware(authmiddleware.RequireEditor()))
}

func testReportPage(c fuego.ContextNoBody) (any, error) {
	state := test.LoadLastRunState()
	nav := layout.FromRequest(c.Request())
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	page, err := layout.RenderPage(c, "Reporte de Tests", ui.Page(state, nav.PreviewPrefix))
	if err != nil {
		return nil, err
	}
	return nil, page.Render(c.Context(), c.Response())
}
