package home

import (
	homeui "gitinittest5/internal/home/ui"
	"gitinittest5/internal/shared/server"
	topologyapp "gitinittest5/internal/topology/application"
	"gitinittest5/internal/ui/layout"
)

func homeHandler(s *server.Server, topology topologyapp.SnapshotReader) {
	server.All(s, "/", func(c server.ContextNoBody) (any, error) {
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
