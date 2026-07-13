package auth

import (
	authui "fixtests1/internal/auth/ui"
	mounted "fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
	"fixtests1/internal/ui/layout"
)

func registerAuthLoginPage(s *server.Server) {
	server.Get(s, "/auth/login", func(c server.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPublicPage(c, "Sync 4 Run Login", authui.LoginPage(mounted.Prefix(c.Request())))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Request().Context(), c.Response())
	})
}
