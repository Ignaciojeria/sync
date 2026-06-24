package home

import "app-mobile-downloader/internal/shared/server"

// Register wires home routes onto the shared server.
func Register(s *server.Server) {
	homeHandler(s)
}
