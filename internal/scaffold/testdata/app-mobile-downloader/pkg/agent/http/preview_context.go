package agent

import (
	"net/http"
	"strings"

	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func previewContextHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Get(s.Server, "/agent/preview-context", getPreviewContext(manager), fuego.OptionMiddleware(requireEditor))
}

func getPreviewContext(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		ownerID := previewOwnerSessionIDFromMountPrefix(currentPreviewPrefixFromRequest(c.Request()))
		if ownerID == "" {
			return map[string]any{"active": false}, nil
		}
		session, err := manager.Get(c.Context(), ownerID)
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{
			"active":        true,
			"sessionId":     session.ID,
			"applicable":    previewApplicable(session),
			"sourcePath":    session.SourcePath,
			"baseBranch":    session.BaseBranch,
			"previewBranch": session.Branch,
		}, nil
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
