package auth

import (
	"scaffoldxd1/internal/shared/configuration"
	"scaffoldxd1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerAuthLoginGoogle(s *server.Server, conf configuration.Conf) {
	fuego.Get(s.Server, "/auth/login/google", func(c fuego.ContextNoBody) (any, error) {
		return startGoogleLogin(c, conf, true)
	})
}
