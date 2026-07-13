package topology

import (
	"fixtests1/internal/shared/server"
	topologyapp "fixtests1/internal/topology/application"
)

func Register(s *server.Server, service *topologyapp.Service) {
	upsertSyncSessionHandler(s, service)
}
