package agent

import (
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/server"
	"net/http"
)

func abortHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "POST /agent/sessions/{id}/abort", requireEditor(abortSession(manager)))
}

func abortSession(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if err := manager.Abort(r.Context(), id); err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
