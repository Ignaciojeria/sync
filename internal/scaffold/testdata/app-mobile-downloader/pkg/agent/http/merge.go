package agent

import (
	"net/http"

	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func mergeHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Post(s.Server, "/agent/sessions/{id}/merge", mergePreview(manager), fuego.OptionMiddleware(requireEditor))
}

func mergePreview(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		result, err := manager.MergePreview(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{
			"ok":            true,
			"baseBranch":    result.BaseBranch,
			"previewBranch": result.PreviewBranch,
			"commit":        result.Commit,
		}, nil
	}
}
