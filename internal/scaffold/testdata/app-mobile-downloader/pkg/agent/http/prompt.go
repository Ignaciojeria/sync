package agent

import (
	"net/http"
	"strings"

	agentapp "app-mobile-downloader/pkg/agent/application"
	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func promptHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Post(s.Server, "/agent/sessions/{id}/prompt", sendPrompt(manager), mw)
	fuego.Post(s.Server, "/agent/sessions/{id}/steer", sendSteer(manager), mw)
}

func sendPrompt(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return sendMessage(manager, func(ctx fuego.ContextNoBody, id string, message string) error {
		return manager.Prompt(ctx.Context(), id, message)
	})
}

func sendSteer(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return sendMessage(manager, func(ctx fuego.ContextNoBody, id string, message string) error {
		return manager.Steer(ctx.Context(), id, message)
	})
}

func sendMessage(manager agentapp.AgentService, send func(fuego.ContextNoBody, string, string) error) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		body, err := decodeJSON[messageRequest](c.Request())
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		message := strings.TrimSpace(body.Message)
		if message == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "message is required"}
		}
		if err := send(c, id, message); err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"ok": true}, nil
	}
}
