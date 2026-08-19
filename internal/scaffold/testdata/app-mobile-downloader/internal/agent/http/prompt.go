package agent

import (
	"log"
	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/server"
	"net/http"
	"strings"
)

func promptHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "POST /agent/sessions/{id}/prompt", requireEditor(sendPrompt(manager)))
	server.Handle(s, "POST /agent/sessions/{id}/steer", requireEditor(sendSteer(manager)))
}

func sendPrompt(manager agentapp.AgentService) http.HandlerFunc {
	return sendMessage(manager, func(r *http.Request, id string, body messageRequest) error {
		input := agentapp.PromptInput{
			Message:   body.Message,
			Action:    agentapp.PromptAction(strings.TrimSpace(body.Action)),
			TurnID:    strings.TrimSpace(body.TurnID),
			UserEmail: emailFromContext(r),
		}
		// ponytail: persistimos el user_prompt ANTES de mandar al
		// runtime. Si PromptRequest falla (timeout, ya procesando,
		// runtime caído, queuePendingInput), el user prompt
		// igualmente queda en el transcript + journal. Antes
		// la persistencia estaba acoplada al success path:
		// PromptRequest llamaba MaterializeUserPrompt al FINAL,
		// después de runtime.Prompt. Si runtime.Prompt fallaba,
		// el user prompt se perdía (transcript vacío, journal
		// vacío) y el chat mostraba sólo la respuesta anterior
		// del assistant sin la pregunta que la disparó. Por eso
		// reportabas "se pierden mis mensajes más que la respuesta
		// del agente" — la respuesta se persistía reactivamente
		// por el SSE handler, el user prompt no.
		//
		// ponytail2: dedup en appendTranscriptItem via
		// readLastTranscriptLine evita escribir dos veces el mismo
		// user prompt si PromptRequest también lo materializa
		// después (byte-exact match), así que el doble path
		// (sendPrompt + PromptRequest's success path) es seguro.
		agentapp.MaterializeUserPrompt(id, body.Message)
		if err := journalPersistUserPrompt(r.Context(), id, body.Message); err != nil {
			log.Printf("agent prompt: failed to persist user_prompt to journal for session %s: %v", truncate(id, 8), err)
			return &journalPersistError{cause: err}
		}
		// Ahora intentamos mandarlo al runtime. Si falla, el user
		// prompt YA está persistido y aparecerá en el transcript
		// en el próximo LoadConversationHistory (sea inmediato o
		// tras un refresh).
		if err := manager.PromptRequest(r.Context(), id, input); err != nil {
			// No removemos el item ya escrito — el user prompt es
			// la intención del usuario, debe quedar registrada
			// aunque el runtime no pueda procesarlo.
			log.Printf("agent prompt: runtime.Prompt failed for session %s: %v (user prompt already persisted)", truncate(id, 8), err)
			return nil
		}
		return nil
	})
}

// journalPersistError envuelve un fallo de journal write para
// que mapSessionError (en support.go) lo traduzca a 503 sin
// filtrar detalles internos al cliente. El handler lo devuelve
// antes del writeJSON(200) de sendMessage, así que el cliente ve
// el status correcto.
type journalPersistError struct{ cause error }

func (e *journalPersistError) Error() string {
	if e.cause == nil {
		return "failed to persist prompt to journal"
	}
	return e.cause.Error()
}

func (e *journalPersistError) Unwrap() error { return e.cause }

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
