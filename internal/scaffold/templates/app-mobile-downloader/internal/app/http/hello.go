package app

import (
	appui "app-mobile-downloader/internal/app/ui"
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(helloWorldHandler)

func helloWorldHandler(s *server.Server) {
	fuego.All(s.Server, "/", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, appui.HomePage().Render(c.Request().Context(), c.Response())
	})
}
