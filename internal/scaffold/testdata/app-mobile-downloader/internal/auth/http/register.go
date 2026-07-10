package auth

import (
	authpostgresql "scaffoldxd1/internal/auth/infrastructure/postgresql"
	"scaffoldxd1/internal/shared/configuration"
	"scaffoldxd1/internal/shared/server"

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
