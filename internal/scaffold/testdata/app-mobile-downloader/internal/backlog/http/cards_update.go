package http

import (
	"strings"

	backlogapp "gitinittest5/internal/backlog/application"
	backlogui "gitinittest5/internal/backlog/ui"
	"gitinittest5/internal/shared/server"
)

func updateCardHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Post(s, "/backlog/cards/{slug}/update", func(c server.ContextNoBody) (any, error) {
		slug := strings.TrimSpace(c.PathParam("slug"))
		if slug == "" {
			return nil, server.HTTPError{Status: 400, Detail: "missing card slug"}
		}
		if err := c.Request().ParseForm(); err != nil {
			return nil, server.HTTPError{Status: 400, Detail: err.Error()}
		}
		title := c.Request().FormValue("title")
		description := c.Request().FormValue("description")
		card, err := svc.Update(c.Context(), slug, title, description)
		if err != nil {
			return nil, translateError(err)
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		if err := backlogui.RenderCardForReplace(c.Response(), c.Context(), card, extractPreviewPrefix(c)); err != nil {
			return nil, err
		}
		return nil, nil
	}, mw)
}
