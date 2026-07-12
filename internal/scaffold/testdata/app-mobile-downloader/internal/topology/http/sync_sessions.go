package topology

import (
	"net/http"

	"testboi1/internal/shared/server"
	topologyapp "testboi1/internal/topology/application"

	"github.com/go-fuego/fuego"
)

type upsertSyncSessionResponse struct {
	OK bool `json:"ok"`
}

func upsertSyncSessionHandler(s *server.Server, service *topologyapp.Service) {
	fuego.Post(s.Server, "/api/topology/sync-sessions", func(c fuego.ContextWithBody[topologyapp.UpsertSyncSessionInput]) (*upsertSyncSessionResponse, error) {
		body, err := c.Body()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		if err := service.UpsertSyncSession(c.Context(), body); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}
		return &upsertSyncSessionResponse{OK: true}, nil
	})
}
