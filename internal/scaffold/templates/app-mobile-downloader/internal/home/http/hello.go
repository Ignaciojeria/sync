package home

import (
	homeui "app-mobile-downloader/internal/home/ui"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func homeHandler(s *server.Server) {
	fuego.All(s.Server, "/", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Home", homeui.HomePage())
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
