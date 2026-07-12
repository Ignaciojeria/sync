package auth

import (
	authpostgresql "testboi1/internal/auth/infrastructure/postgresql"
	"testboi1/internal/shared/configuration"
	"testboi1/internal/shared/server"

	"github.com/MicahParks/keyfunc/v3"
)

// Register wires every auth route onto the shared server.
func Register(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository, jwks keyfunc.Keyfunc) {
	registerStaticAssets(s)
	registerAuthLoginPage(s)
	registerAuthLoginGoogle(s, conf)
	registerAuthCallback(s, conf, store, jwks)
	registerAuthLogout(s, conf, store)
}
