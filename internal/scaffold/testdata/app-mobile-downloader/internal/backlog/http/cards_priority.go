package http

import (
	"encoding/json"
	"strconv"
	"strings"

	backlogapp "gitinittest5/internal/backlog/application"
	backlogui "gitinittest5/internal/backlog/ui"
	"gitinittest5/internal/shared/server"
)

func priorityCardHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Post(s, "/backlog/cards/{slug}/priority", func(c server.ContextNoBody) (any, error) {
		slug := strings.TrimSpace(c.PathParam("slug"))
		if slug == "" {
			return nil, server.HTTPError{Status: 400, Detail: "missing card slug"}
		}

		var body struct {
			Delta    *int    `json:"delta,omitempty"`
			Priority *string `json:"priority,omitempty"`
		}
		if c.Request().ContentLength > 0 {
			if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
				return nil, server.HTTPError{Status: 400, Detail: "invalid body: " + err.Error()}
			}
		} else if err := c.Request().ParseForm(); err == nil {
			if v, err := strconv.Atoi(c.Request().FormValue("delta")); err == nil {
				body.Delta = &v
			}
			if p := c.Request().FormValue("priority"); p != "" {
				body.Priority = &p
			}
		}

		cur, err := svc.Get(c.Context(), slug)
		if err != nil {
			return nil, translateError(err)
		}

		var newP backlogapp.Priority
		switch {
		case body.Priority != nil && *body.Priority != "":
			newP = backlogapp.Priority(*body.Priority)
		case body.Delta != nil:
			// Parsear priority actual (P0..P3) → int → aplicar delta → string.
			curInt, _ := strconv.Atoi(strings.TrimPrefix(string(cur.Priority), "P"))
			newInt := curInt + *body.Delta
			newP = backlogapp.Priority("P" + strconv.Itoa(newInt))
		default:
			return nil, server.HTTPError{Status: 400, Detail: "missing priority or delta"}
		}

		// Saturar (no fallar) en los extremos.
		if !newP.Valid() {
			if newP < backlogapp.PriorityP0 {
				newP = backlogapp.PriorityP0
			} else {
				newP = backlogapp.PriorityP3
			}
		}

		updated, err := svc.SetPriority(c.Context(), slug, newP)
		if err != nil {
			return nil, translateError(err)
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		if err := backlogui.RenderCardForReplace(c.Response(), c.Context(), updated, extractPreviewPrefix(c)); err != nil {
			return nil, err
		}
		return nil, nil
	}, mw)
}
