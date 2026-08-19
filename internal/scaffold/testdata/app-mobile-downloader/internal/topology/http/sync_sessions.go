package topology

import (
	"encoding/json"
	"gitinittest5/internal/shared/server"
	topologyapp "gitinittest5/internal/topology/application"
	"net/http"
)

type upsertSyncSessionResponse struct {
	OK bool `json:"ok"`
}

func upsertSyncSessionHandler(s *server.Server, service *topologyapp.Service) {
	server.Post(s, "/api/topology/sync-sessions", func(c server.ContextNoBody) (any, error) {
		var body topologyapp.UpsertSyncSessionInput
		if err := json.NewDecoder(c.Request().Body).Decode(&body); err != nil {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		if err := service.UpsertSyncSession(c.Context(), body); err != nil {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		return &upsertSyncSessionResponse{OK: true}, nil
	})
}
