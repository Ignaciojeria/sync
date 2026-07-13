package home

import (
	"fixtests1/internal/shared/server"
	topologyapp "fixtests1/internal/topology/application"
)

// Register wires home routes onto the shared server.
func Register(s *server.Server, topology topologyapp.SnapshotReader) {
	homeHandler(s, topology)
}
