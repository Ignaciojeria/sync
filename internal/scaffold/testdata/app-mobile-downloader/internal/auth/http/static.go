package auth

import (
	"gitinittest5/internal/shared/server"
)

func registerStaticAssets(s *server.Server) {
	server.Get(s, "/logo.jpeg", serveStaticFile("logo.jpeg"))
	server.Get(s, "/logo.svg", serveStaticFile("logo.svg"))
	server.Get(s, "/login.jpeg", serveStaticFile("login.jpeg"))
	server.Get(s, "/login-bg.svg", serveStaticFile("login-bg.svg"))
}
