package topology

import (
	"scaffoldxd1/internal/shared/server"
	topologyapp "scaffoldxd1/internal/topology/application"
)

func Register(s *server.Server, service *topologyapp.Service) {
	upsertSyncSessionHandler(s, service)
}
