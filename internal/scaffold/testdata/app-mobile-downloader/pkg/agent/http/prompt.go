package agent

import (
	"net/http"
	"strconv"
	"strings"

	"scaffoldxd1/internal/shared/server"
	agentapp "scaffoldxd1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func promptHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Post(s.Server, "/agent/sessions/{id}/prompt", sendPrompt(manager), mw)
	fuego.Post(s.Server, "/agent/sessions/{id}/steer", sendSteer(manager), mw)
}

func sendPrompt(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return sendMessage(manager, func(ctx fuego.ContextNoBody, id string, body messageRequest) error {
		input := agentapp.PromptInput{
			Message: body.Message,
			Action:  agentapp.PromptAction(strings.TrimSpace(body.Action)),
			TurnID:  strings.TrimSpace(body.TurnID),
		}
		if err := manager.PromptRequest(ctx.Context(), id, input); err != nil {
			return err
		}
		if runtimeEventsStore != nil {
			event := agentapp.Event{
				SessionID: id,
				Type:      "user_prompt",
				Payload:   []byte(`{"text":` + strconv.Quote(strings.TrimSpace(body.Message)) + `}`),
			}
			if _, err := runtimeEventsStore.Append(ctx.Context(), id, "pi", event); err != nil {
				// ponytail: user prompt still persists in legacy transcript; postgres append is best-effort until legacy is removed.
			}
		}
		return nil
	})
}

func sendSteer(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return sendMessage(manager, func(ctx fuego.ContextNoBody, id string, body messageRequest) error {
		return manager.Steer(ctx.Context(), id, strings.TrimSpace(body.Message))
	})
}

func sendMessage(manager agentapp.AgentService, send func(fuego.ContextNoBody, string, messageRequest) error) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		body, err := decodeJSON[messageRequest](c.Request())
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		body.Message = strings.TrimSpace(body.Message)
		body.Action = strings.TrimSpace(body.Action)
		body.TurnID = strings.TrimSpace(body.TurnID)
		if body.Message == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "message is required"}
		}
		if err := send(c, id, body); err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"ok": true}, nil
	}
}
