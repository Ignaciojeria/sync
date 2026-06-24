package auth

import (
	authui "app-mobile-downloader/internal/auth/ui"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func registerAuthLoginPage(s *server.Server) {
	fuego.Get(s.Server, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPublicPage(c, "Sync 4 Run Login", authui.LoginPage())
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Request().Context(), c.Response())
	})
}
