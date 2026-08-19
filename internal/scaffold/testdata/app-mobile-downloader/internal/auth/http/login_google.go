package auth

import (
	"gitinittest5/internal/shared/configuration"
	"gitinittest5/internal/shared/server"
)

func registerAuthLoginGoogle(s *server.Server, conf configuration.Conf) {
	server.Get(s, "/auth/login/google", func(c server.ContextNoBody) (any, error) {
		return startGoogleLogin(c, conf, true)
	})
}
