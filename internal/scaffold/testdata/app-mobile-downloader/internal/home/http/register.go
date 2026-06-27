package home

import (
	"app-mobile-downloader/internal/shared/server"
	topologyapp "app-mobile-downloader/internal/topology/application"
)

// Register wires home routes onto the shared server.
func Register(s *server.Server, topology topologyapp.SnapshotReader) {
	homeHandler(s, topology)
}
