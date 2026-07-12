package editor

import "testboi1/internal/shared/server"

// Register wires editor routes (reverse proxy + console view) onto the server.
func Register(s *server.Server) {
	editorHandler(s)
	editorViewHandler(s)
}
