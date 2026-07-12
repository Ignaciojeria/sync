package agent

import (
	"net/http"

	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func applyHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Post(s.Server, "/agent/sessions/{id}/apply", applyPreview(manager), fuego.OptionMiddleware(requireEditor))
}

func applyPreview(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		result, err := manager.ApplyPreview(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{
			"ok":          true,
			"sourcePath":  result.SourcePath,
			"previewPath": result.PreviewPath,
		}, nil
	}
}
