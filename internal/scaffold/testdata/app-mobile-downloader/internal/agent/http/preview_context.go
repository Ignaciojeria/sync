package agent

import (
	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/server"
	"net/http"
	"strings"
)

func previewContextHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/preview-context", requireEditor(getPreviewContext(manager)))
}

func getPreviewContext(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ownerID := previewOwnerSessionIDFromMountPrefix(currentPreviewPrefixFromRequest(r))
		if ownerID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"active": false})
			return
		}
		session, err := manager.Get(r.Context(), ownerID)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"active":        true,
			"sessionId":     session.ID,
			"applicable":    previewApplicable(session),
			"sourcePath":    session.SourcePath,
			"baseBranch":    session.BaseBranch,
			"previewBranch": session.Branch,
		})
	}
}

func previewApplicable(session agentapp.Session) bool {
	if strings.TrimSpace(session.ID) == "" {
		return false
	}
	if strings.TrimSpace(session.WorkspacePath) == "" || strings.TrimSpace(session.SourcePath) == "" {
		return false
	}
	return true
}
