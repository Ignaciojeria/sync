package auth

import (
	authui "scaffoldxd1/internal/auth/ui"
	mounted "scaffoldxd1/internal/shared/mounted"
	"scaffoldxd1/internal/shared/server"
	"scaffoldxd1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func registerAuthLoginPage(s *server.Server) {
	fuego.Get(s.Server, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPublicPage(c, "Sync 4 Run Login", authui.LoginPage(mounted.Prefix(c.Request())))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Request().Context(), c.Response())
	})
}
