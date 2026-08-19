package topology

import (
	"gitinittest5/internal/shared/server"
	topologyapp "gitinittest5/internal/topology/application"
)

func Register(s *server.Server, service *topologyapp.Service) {
	upsertSyncSessionHandler(s, service)
}
