package agent

import (
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
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
		session, err := manager.Create(r.Context(), agentapp.CreateSessionInput{Title: body.Title, CWD: body.CWD, Model: body.Model})
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
