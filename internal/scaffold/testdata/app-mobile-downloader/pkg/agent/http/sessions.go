package agent

import (
	"net/http"

	agentapp "app-mobile-downloader/pkg/agent/application"
	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func sessionsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Get(s.Server, "/agent/sessions", listSessions(manager), mw)
	fuego.Post(s.Server, "/agent/sessions", createSession(manager), mw)
	fuego.Get(s.Server, "/agent/sessions/{id}", getSession(manager), mw)
}

func listSessions(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		sessions, err := manager.List(c.Context())
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"sessions": sessions}, nil
	}
}

func createSession(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		body, err := decodeJSON[createSessionRequest](c.Request())
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		session, err := manager.Create(c.Context(), agentapp.CreateSessionInput{
			Title: body.Title,
			CWD:   body.CWD,
			Model: body.Model,
		})
		if err != nil {
			return nil, mapSessionError(err)
		}
		c.Response().Header().Set("Location", "/agent?session="+session.ID)
		return map[string]any{"session": session}, nil
	}
}

func getSession(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		session, err := manager.Get(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"session": session}, nil
	}
}
