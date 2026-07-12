package agent

import (
	"net/http"

	"testboi1/internal/shared/mounted"
	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func sessionsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Get(s.Server, "/agent/sessions", listSessions(manager), mw)
	fuego.Post(s.Server, "/agent/sessions", createSession(manager), mw)
	fuego.Get(s.Server, "/agent/sessions/{id}", getSession(manager), mw)
	fuego.Delete(s.Server, "/agent/sessions/{id}", deleteSession(manager), mw)
	fuego.Get(s.Server, "/agent/sessions/{id}/history", getHistory(manager), mw)
}

func listSessions(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		sessions, err := manager.List(c.Context())
		if err != nil {
			return nil, mapSessionError(err)
		}
		if ownerID := previewOwnerSessionIDFromRequest(c.Request()); ownerID != "" {
			filtered := sessions[:0]
			for _, session := range sessions {
				if session.ID == ownerID {
					continue
				}
				filtered = append(filtered, session)
			}
			sessions = filtered
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
		c.Response().Header().Set("Location", mounted.App(mounted.Prefix(c.Request()), "/agent?session="+session.ID))
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

func deleteSession(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		if err := manager.Delete(c.Context(), id); err != nil {
			return nil, mapSessionError(err)
		}
		c.Response().WriteHeader(http.StatusNoContent)
		return nil, nil
	}
}

func getHistory(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		if _, err := manager.Get(c.Context(), id); err != nil {
			return nil, mapSessionError(err)
		}
		history, err := agentapp.LoadConversationHistoryCtx(
			c.Context(),
			id,
			agentapp.ParseHistoryBefore(c.QueryParam("before")),
			agentapp.ParseHistoryLimit(c.QueryParam("limit"), 30),
		)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		return history, nil
	}
}
