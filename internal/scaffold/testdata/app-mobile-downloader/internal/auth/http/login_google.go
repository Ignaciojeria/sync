package auth

import (
	"testboi1/internal/shared/configuration"
	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerAuthLoginGoogle(s *server.Server, conf configuration.Conf) {
	fuego.Get(s.Server, "/auth/login/google", func(c fuego.ContextNoBody) (any, error) {
		return startGoogleLogin(c, conf, true)
	})
}
