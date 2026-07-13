package auth

import (
	"fixtests1/internal/shared/configuration"
	"fixtests1/internal/shared/server"
)

func registerAuthLoginGoogle(s *server.Server, conf configuration.Conf) {
	server.Get(s, "/auth/login/google", func(c server.ContextNoBody) (any, error) {
		return startGoogleLogin(c, conf, true)
	})
}
