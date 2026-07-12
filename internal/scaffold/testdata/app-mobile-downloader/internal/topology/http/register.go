package topology

import (
	"testboi1/internal/shared/server"
	topologyapp "testboi1/internal/topology/application"
)

func Register(s *server.Server, service *topologyapp.Service) {
	upsertSyncSessionHandler(s, service)
}
