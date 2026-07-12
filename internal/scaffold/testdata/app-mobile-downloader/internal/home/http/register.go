package home

import (
	"testboi1/internal/shared/server"
	topologyapp "testboi1/internal/topology/application"
)

// Register wires home routes onto the shared server.
func Register(s *server.Server, topology topologyapp.SnapshotReader) {
	homeHandler(s, topology)
}
