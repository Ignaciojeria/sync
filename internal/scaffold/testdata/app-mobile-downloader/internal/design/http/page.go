package design

import (
	designapp "testboi1/internal/design/application"
	designui "testboi1/internal/design/ui"
	"testboi1/internal/shared/server"
	"testboi1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func registerPageHandler(s *server.Server, catalog designapp.Catalog) {
	fuego.Get(s.Server, "/design", designPageHandler(catalog))
}

func designPageHandler(catalog designapp.Catalog) func(c fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
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
