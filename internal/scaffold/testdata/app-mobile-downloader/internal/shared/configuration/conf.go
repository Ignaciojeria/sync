package configuration

type Conf struct {
	// PORT default 8001: el web-server NO es público, está detrás del BFF.
	// Para deploys sin BFF (legacy), setear PORT=8000 manualmente.
	PORT         string `env:"PORT" envDefault:"8000"`
	PROJECT_NAME string `env:"PROJECT_NAME"`

	DATABASE_URL string `env:"DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"`

	OIDCIssuer                 string `env:"OIDC_ISSUER"`
	OIDCJWKSURI                string `env:"OIDC_JWKS_URI"`
	OIDCAuthorizationEndpoint  string `env:"OIDC_AUTHORIZATION_ENDPOINT"`
	OIDCTokenEndpoint          string `env:"OIDC_TOKEN_ENDPOINT"`
	OIDCClientID               string `env:"OIDC_CLIENT_ID"`
	OIDCClientSecret           string `env:"OIDC_CLIENT_SECRET"`
	OIDCRedirectURI            string `env:"OIDC_REDIRECT_URI"`
	OIDCScopes                 string `env:"OIDC_SCOPES" envDefault:"openid profile email"`
	OIDCLoginURL               string `env:"OIDC_LOGIN_URL"`
	OIDCGoogleLoginURL         string `env:"OIDC_GOOGLE_LOGIN_URL"`
	OIDCUpstreamGoogleClientID string `env:"OIDC_UPSTREAM_GOOGLE_CLIENT_ID"`

	JWKSURLS    string `env:"JWKS_URLS"`
	JWTAudience string `env:"JWT_AUDIENCE"`

	// sync-ai-gateway.exe.xyz — mismo gateway que usa pi (.pi/extensions/provider.ts).
	// Mismo par de env vars, misma convención. La key NUNCA sale al browser:
	// el server la usa server-side para proxy a /balance y mostrarlo en la UI.
	SyncAIGatewayBaseURL string `env:"SYNC_AI_GATEWAY_BASE_URL" envDefault:"https://sync-ai-gateway.exe.xyz/api/gateway/v1"`
	SyncAIGatewayAPIKey  string `env:"SYNC_AI_GATEWAY_API_KEY"`
}

func NewConf() (Conf, error) {
	return Parse[Conf]()
}
