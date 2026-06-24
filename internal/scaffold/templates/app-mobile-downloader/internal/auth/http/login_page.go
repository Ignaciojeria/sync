package auth

import (
	authui "app-mobile-downloader/internal/auth/ui"
	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerAuthLoginPage(s *server.Server) {
	fuego.Get(s.Server, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, authui.LoginPage().Render(c.Request().Context(), c.Response())
	})
}
