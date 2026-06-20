package auth

import (
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(registerAuthLoginGoogle)

func registerAuthLoginGoogle(s *server.Server, conf configuration.Conf) {
	fuego.Get(s.Server, "/auth/login/google", func(c fuego.ContextNoBody) (any, error) {
		return startGoogleLogin(c, conf, true)
	})
}
