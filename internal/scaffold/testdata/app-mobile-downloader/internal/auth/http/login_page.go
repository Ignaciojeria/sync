package auth

import (
	authui "gitinittest5/internal/auth/ui"
	mounted "gitinittest5/internal/shared/mounted"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"
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
