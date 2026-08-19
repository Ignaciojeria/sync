package home

import (
	"gitinittest5/internal/shared/server"
	topologyapp "gitinittest5/internal/topology/application"
)

// Register wires home routes onto the shared server.
func Register(s *server.Server, topology topologyapp.SnapshotReader) {
	homeHandler(s, topology)
}
