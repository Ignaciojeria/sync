package topology

import (
	"app-mobile-downloader/internal/shared/server"
	topologyapp "app-mobile-downloader/internal/topology/application"
)

func Register(s *server.Server, service *topologyapp.Service) {
	upsertSyncSessionHandler(s, service)
}
