package http

import (
	backlogapp "gitinittest5/internal/backlog/application"
	"gitinittest5/internal/shared/server"
)

func deleteCardHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Delete(s, "/backlog/cards/{slug}", func(c server.ContextNoBody) (any, error) {
		slug := c.PathParam("slug")
		if slug == "" {
			return nil, server.HTTPError{Status: 400, Detail: "missing card slug"}
		}
		if err := svc.Delete(c.Context(), slug); err != nil {
			return nil, translateError(err)
		}
		// El cliente usa hx-swap="outerHTML swap:1s" para animar la
		// salida; respondemos vacío para que el nodo se retire.
		return nil, nil
	}, mw)
}
