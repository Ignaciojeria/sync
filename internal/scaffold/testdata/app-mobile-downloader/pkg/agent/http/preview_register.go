package agent

import (
	"net"
	"net/http"
	"strings"

	"scaffoldxd1/internal/shared/server"
	agentapp "scaffoldxd1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func previewControlHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Post(s.Server, "/agent/sessions/{id}/preview", registerPreview(manager), mw)
	fuego.Delete(s.Server, "/agent/sessions/{id}/preview", clearPreview(manager), mw)

	internalMW := fuego.OptionMiddleware(requireLoopback())
	fuego.Post(s.Server, "/agent/internal/sessions/{id}/preview", registerPreview(manager), internalMW)
	fuego.Delete(s.Server, "/agent/internal/sessions/{id}/preview", clearPreview(manager), internalMW)
}

func registerPreview(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		body, err := decodeJSON[previewRequest](c.Request())
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		session, err := manager.RegisterPreview(c.Context(), id, agentapp.RegisterPreviewInput{
			Port:       body.Port,
			HealthPath: body.HealthPath,
		})
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"session": session}, nil
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

func clearPreview(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		session, err := manager.ClearPreview(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		return map[string]any{"session": session}, nil
	}
}
