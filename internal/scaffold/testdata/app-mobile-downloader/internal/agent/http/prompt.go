package agent

import (
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/server"
	"net/http"
	"strconv"
	"strings"
)

func promptHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "POST /agent/sessions/{id}/prompt", requireEditor(sendPrompt(manager)))
	server.Handle(s, "POST /agent/sessions/{id}/steer", requireEditor(sendSteer(manager)))
}

func sendPrompt(manager agentapp.AgentService) http.HandlerFunc {
	return sendMessage(manager, func(r *http.Request, id string, body messageRequest) error {
		input := agentapp.PromptInput{
			Message: body.Message,
			Action:  agentapp.PromptAction(strings.TrimSpace(body.Action)),
			TurnID:  strings.TrimSpace(body.TurnID),
		}
		if err := manager.PromptRequest(r.Context(), id, input); err != nil {
			return err
		}
		if runtimeEventsStore != nil {
			event := agentapp.Event{SessionID: id, Type: "user_prompt", Payload: []byte(`{"text":` + strconv.Quote(strings.TrimSpace(body.Message)) + `}`)}
			if _, err := runtimeEventsStore.Append(r.Context(), id, "pi", event); err != nil {
				// ponytail: user prompt still persists in legacy transcript; postgres append is best-effort until legacy is removed.
			}
		}
		return nil
	})
}

func sendSteer(manager agentapp.AgentService) http.HandlerFunc {
	return sendMessage(manager, func(r *http.Request, id string, body messageRequest) error {
		return manager.Steer(r.Context(), id, strings.TrimSpace(body.Message))
	})
}

func sendMessage(manager agentapp.AgentService, send func(*http.Request, string, messageRequest) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		body, err := decodeJSON[messageRequest](r)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()})
			return
		}
		body.Message = strings.TrimSpace(body.Message)
		body.Action = strings.TrimSpace(body.Action)
		body.TurnID = strings.TrimSpace(body.TurnID)
		if body.Message == "" {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: "message is required"})
			return
		}
		if err := send(r, id, body); err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
