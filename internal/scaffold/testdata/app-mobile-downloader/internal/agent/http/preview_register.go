package agent

import (
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/server"
	"net"
	"net/http"
	"strings"
)

func previewControlHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "POST /agent/sessions/{id}/preview", requireEditor(registerPreview(manager)))
	server.Handle(s, "DELETE /agent/sessions/{id}/preview", requireEditor(clearPreview(manager)))

	internalMW := requireLoopback()
	server.Handle(s, "POST /agent/internal/sessions/{id}/preview", internalMW(registerPreview(manager)))
	server.Handle(s, "DELETE /agent/internal/sessions/{id}/preview", internalMW(clearPreview(manager)))
}

func registerPreview(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		body, err := decodeJSON[previewRequest](r)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()})
			return
		}
		session, err := manager.RegisterPreview(r.Context(), id, agentapp.RegisterPreviewInput{Port: body.Port, HealthPath: body.HealthPath})
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": session})
	}
}

func requireLoopback() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.TrimSpace(r.RemoteAddr)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				http.Error(w, "loopback only", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clearPreview(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		session, err := manager.ClearPreview(r.Context(), id)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"session": session})
	}
}
