package http

import (
	backlogapp "gitinittest5/internal/backlog/application"
	backlogui "gitinittest5/internal/backlog/ui"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"
)

func boardPageHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Get(s, "/backlog", func(c server.ContextNoBody) (any, error) {
		board, err := svc.Board(c.Context())
		if err != nil {
			return nil, err
		}
		nav := layout.FromRequest(c.Request())
		page, err := layout.RenderPage(c, "Backlog", backlogui.Page(backlogui.ToBoardView(board), nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	}, mw)
}
