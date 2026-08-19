package agent

import (
	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/server"
	"net/http"
)

func mergeHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "POST /agent/sessions/{id}/merge", requireEditor(mergePreview(manager)))
}

func mergePreview(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		result, err := manager.MergePreview(r.Context(), id)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"baseBranch":    result.BaseBranch,
			"previewBranch": result.PreviewBranch,
			"commit":        result.Commit,
		})
	}
}
