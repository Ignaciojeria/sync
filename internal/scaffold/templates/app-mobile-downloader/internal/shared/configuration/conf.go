package configuration

import (
	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewConf)

type Conf struct {
	PORT         string `env:"PORT" envDefault:"8000"`
	PROJECT_NAME string `env:"PROJECT_NAME"`
	VERSION      string `env:"VERSION"`

	DATABASE_URL string `env:"DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"`

	OIDCType                 string `env:"OIDC_TYPE" envDefault:"oidc"`
	OIDCProvider             string `env:"OIDC_PROVIDER" envDefault:"casdoor"`
	OIDCIssuer               string `env:"OIDC_ISSUER"`
	OIDCDiscoveryURL         string `env:"OIDC_DISCOVERY_URL"`
	OIDCJWKSURI              string `env:"OIDC_JWKS_URI"`
	OIDCAuthorizationEndpoint string `env:"OIDC_AUTHORIZATION_ENDPOINT"`
	OIDCTokenEndpoint        string `env:"OIDC_TOKEN_ENDPOINT"`
	OIDCUserinfoEndpoint     string `env:"OIDC_USERINFO_ENDPOINT"`
	OIDCClientID             string `env:"OIDC_CLIENT_ID"`
	OIDCClientSecret         string `env:"OIDC_CLIENT_SECRET"`
	OIDCClientSecretRef      string `env:"OIDC_CLIENT_SECRET_REF"`
	OIDCRedirectURI          string `env:"OIDC_REDIRECT_URI"`
	OIDCLogoutURI            string `env:"OIDC_LOGOUT_URI"`
	OIDCPostLogoutRedirectURI string `env:"OIDC_POST_LOGOUT_REDIRECT_URI"`
	OIDCScopes               string `env:"OIDC_SCOPES" envDefault:"openid profile email"`
	OIDCLoginURL             string `env:"OIDC_LOGIN_URL"`
	OIDCGoogleLoginURL       string `env:"OIDC_GOOGLE_LOGIN_URL"`
	OIDCUpstreamGoogleClientID string `env:"OIDC_UPSTREAM_GOOGLE_CLIENT_ID"`

	MachineAuthGrantType      string `env:"MACHINE_AUTH_GRANT_TYPE" envDefault:"client_credentials"`
	MachineAuthTokenEndpoint  string `env:"MACHINE_AUTH_TOKEN_ENDPOINT"`
	MachineAuthClientID       string `env:"MACHINE_AUTH_CLIENT_ID"`
	MachineAuthClientSecret   string `env:"MACHINE_AUTH_CLIENT_SECRET"`
	MachineAuthClientSecretRef string `env:"MACHINE_AUTH_CLIENT_SECRET_REF"`
	MachineAuthAudience       string `env:"MACHINE_AUTH_AUDIENCE"`
	MachineAuthScopes         string `env:"MACHINE_AUTH_SCOPES"`

	AUTH_DISABLED string `env:"AUTH_DISABLED" envDefault:"false"`
	JWKSURLS      string `env:"JWKS_URLS"`
	JWTAudience   string `env:"JWT_AUDIENCE"`
}

func NewConf() (Conf, error) {
	return Parse[Conf]()
}
