package home

import (
	homeui "testboi1/internal/home/ui"
	"testboi1/internal/shared/server"
	topologyapp "testboi1/internal/topology/application"
	"testboi1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func homeHandler(s *server.Server, topology topologyapp.SnapshotReader) {
	fuego.All(s.Server, "/", func(c fuego.ContextNoBody) (any, error) {
		snapshot, err := topology.GetSnapshot(c.Context())
		if err != nil {
			return nil, err
		}

		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Home", homeui.HomePage(snapshot, nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
