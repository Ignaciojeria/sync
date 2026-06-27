package home

import (
	homeui "app-mobile-downloader/internal/home/ui"
	"app-mobile-downloader/internal/shared/server"
	topologyapp "app-mobile-downloader/internal/topology/application"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func homeHandler(s *server.Server, topology topologyapp.SnapshotReader) {
	fuego.All(s.Server, "/", func(c fuego.ContextNoBody) (any, error) {
		snapshot, err := topology.GetSnapshot(c.Context())
		if err != nil {
			return nil, err
		}

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Home", homeui.HomePage(snapshot))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
