package design

import (
	designapp "gitinittest5/internal/design/application"
	designui "gitinittest5/internal/design/ui"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"
)

func registerPageHandler(s *server.Server, catalog designapp.Catalog) {
	server.Get(s, "/design", designPageHandler(catalog))
}

func designPageHandler(catalog designapp.Catalog) func(c server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		nav := layout.FromRequest(c.Request())
		activeThemeID := catalog.ActiveThemeIDFromRequest(c.Request())
		activeTheme, _ := catalog.ThemeByID(activeThemeID)
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Design System", designui.Page(catalog.Themes(), activeTheme, nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	}
}
