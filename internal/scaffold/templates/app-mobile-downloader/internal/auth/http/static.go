package auth

import (
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(registerStaticAssets)

func registerStaticAssets(s *server.Server) {
	fuego.Get(s.Server, "/logo.jpeg", serveStaticFile("logo.jpeg"))
	fuego.Get(s.Server, "/logo.svg", serveStaticFile("logo.svg"))
	fuego.Get(s.Server, "/login.jpeg", serveStaticFile("login.jpeg"))
	fuego.Get(s.Server, "/login-bg.svg", serveStaticFile("login-bg.svg"))
}
