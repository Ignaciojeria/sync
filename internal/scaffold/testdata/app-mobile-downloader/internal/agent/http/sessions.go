package agent

import (
	agentapp "lastmile-agents/internal/agent/application"
	agentuiv2 "lastmile-agents/internal/agent/ui/v2"
	"lastmile-agents/internal/shared/mounted"
	"lastmile-agents/internal/shared/server"
	"net/http"
	"strings"
)

func sessionsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/sessions", requireEditor(listSessions(manager)))
	server.Handle(s, "POST /agent/sessions", requireEditor(createSession(manager)))
	server.Handle(s, "GET /agent/sessions/{id}", requireEditor(getSession(manager)))
	server.Handle(s, "DELETE /agent/sessions/{id}", requireEditor(deleteSession(manager)))
	server.Handle(s, "GET /agent/sessions/{id}/history", requireEditor(getHistory(manager)))
}

func listSessions(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions, err := manager.List(r.Context())
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		if ownerID := previewOwnerSessionIDFromRequest(r); ownerID != "" {
			filtered := sessions[:0]
			for _, session := range sessions {
				if session.ID == ownerID {
					continue
				}
				filtered = append(filtered, session)
			}
			sessions = filtered
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
	}
}

func createSession(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := decodeJSON[createSessionRequest](r)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()})
			return
		}
		input := agentapp.CreateSessionInput{
			Title:      body.Title,
			CWD:        body.CWD,
			Model:      body.Model,
			OwnerEmail: emailFromContext(r),
		}
		session, err := manager.Create(r.Context(), input)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		w.Header().Set("Location", mounted.App(mounted.Prefix(r), "/agent?session="+session.ID))
		writeJSON(w, http.StatusOK, map[string]any{"session": session})
	}
}

func getSession(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		session, err := manager.Get(r.Context(), id)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": session})
	}
}

func deleteSession(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := manager.Delete(r.Context(), id); err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		// ponytail: liberar el renderer per-session atado por el
		// page handler. Sin esto, el registry acumula entradas
		// para sesiones eliminadas (memory leak chico pero
		// acumulativo) y un recreate futuro puede heredar un
		// renderer "stale" apuntando a la sesión vieja.
		agentuiv2.ClearRendererForSession(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

func getHistory(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if _, err := manager.Get(r.Context(), id); err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		history, err := agentapp.LoadConversationHistoryCtx(
			r.Context(),
			id,
			agentapp.ParseHistoryBefore(strings.TrimSpace(r.URL.Query().Get("before"))),
			agentapp.ParseHistoryLimit(strings.TrimSpace(r.URL.Query().Get("limit")), 30),
		)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, history)
	}
}

// getMessagesHTML se removió en el cutover de 2026-07. El
// endpoint /agent/sessions/{id}/messages renderizaba el feed
// inicial server-side con el template V1 (agentui.RenderItem).
// Tras el cutover la página carga la historia directo en el
// render inicial del shell (v2) y el cliente no necesita este
// endpoint. Si más adelante hace falta, escribir el equivalente
// v2 en internal/agent/http/.
