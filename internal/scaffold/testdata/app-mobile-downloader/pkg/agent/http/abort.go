package agent

import (
	"net/http"

	agentapp "scaffoldxd1/pkg/agent/application"
	"scaffoldxd1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func abortHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Post(s.Server, "/agent/sessions/{id}/abort", abortSession(manager), fuego.OptionMiddleware(requireEditor))
}

func abortSession(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		if err := manager.Abort(c.Context(), id); err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"ok": true}, nil
	}
}
