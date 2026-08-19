package http

import (
	"encoding/json"
	"strings"

	backlogapp "gitinittest5/internal/backlog/application"
	backlogui "gitinittest5/internal/backlog/ui"
	"gitinittest5/internal/shared/server"
)

func moveCardHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Post(s, "/backlog/cards/{slug}/move", func(c server.ContextNoBody) (any, error) {
		slug := strings.TrimSpace(c.PathParam("slug"))
		if slug == "" {
			return nil, server.HTTPError{Status: 400, Detail: "missing card slug"}
		}

		var body struct {
			To string `json:"to"`
		}
		if c.Request().ContentLength > 0 {
			if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
				return nil, server.HTTPError{Status: 400, Detail: "invalid body: " + err.Error()}
			}
		} else if err := c.Request().ParseForm(); err == nil {
			body.To = c.Request().FormValue("to")
		}

		to := backlogapp.Status(strings.TrimSpace(body.To))
		card, err := svc.Move(c.Context(), slug, to)
		if err != nil {
			return nil, translateError(err)
		}

		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		if err := backlogui.RenderCardForMove(c.Response(), c.Context(), card, extractPreviewPrefix(c)); err != nil {
			return nil, err
		}
		return nil, nil
	}, mw)
}
