package configuration

import (
	"os"
	"sync"
	"testing"
)

func unsetEnvForTest(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if ok {
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("Unsetenv(%q) error = %v", key, err)
			}
			t.Cleanup(func() {
				_ = os.Setenv(key, value)
			})
		}
	}
}

func TestNewConf(t *testing.T) {
	defer resetParseDeps()
	once = sync.Once{}

	t.Setenv("PORT", "9090")
	t.Setenv("PROJECT_NAME", "app-mobile-downloader")
	t.Setenv("VERSION", "1.2.3")
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("OIDC_ISSUER", "https://issuer.example")
	t.Setenv("OIDC_CLIENT_ID", "client-id")
	t.Setenv("OIDC_TOKEN_ENDPOINT", "https://issuer.example/token")
	t.Setenv("OIDC_CLIENT_SECRET", "secret")
	t.Setenv("JWT_AUDIENCE", "audience")

	conf, err := NewConf()
	if err != nil {
		t.Fatalf("NewConf() error = %v", err)
	}

	if conf.PORT != "9090" {
		t.Fatalf("PORT = %q", conf.PORT)
	}
	if conf.PROJECT_NAME != "app-mobile-downloader" {
		t.Fatalf("PROJECT_NAME = %q", conf.PROJECT_NAME)
	}
	if conf.VERSION != "1.2.3" {
		t.Fatalf("VERSION = %q", conf.VERSION)
	}
	if conf.DATABASE_URL != "postgres://example" {
		t.Fatalf("DATABASE_URL = %q", conf.DATABASE_URL)
	}
	if conf.OIDCIssuer != "https://issuer.example" {
		t.Fatalf("OIDCIssuer = %q", conf.OIDCIssuer)
	}
	if conf.OIDCClientID != "client-id" {
		t.Fatalf("OIDCClientID = %q", conf.OIDCClientID)
	}
	if conf.OIDCTokenEndpoint != "https://issuer.example/token" {
		t.Fatalf("OIDCTokenEndpoint = %q", conf.OIDCTokenEndpoint)
	}
	if conf.OIDCClientSecret != "secret" {
		t.Fatalf("OIDCClientSecret = %q", conf.OIDCClientSecret)
	}
	if conf.JWTAudience != "audience" {
		t.Fatalf("JWTAudience = %q", conf.JWTAudience)
	}
}

func TestConfDefaultsAndGetters(t *testing.T) {
	defer resetParseDeps()
	once = sync.Once{}

	loadDotEnv = func(filenames ...string) error { return nil }
	unsetEnvForTest(t,
		"PORT",
		"DATABASE_URL",
		"OIDC_TYPE",
		"OIDC_PROVIDER",
		"OIDC_SCOPES",
		"MACHINE_AUTH_GRANT_TYPE",
		"AUTH_DISABLED",
	)

	conf, err := NewConf()
	if err != nil {
		t.Fatalf("NewConf() error = %v", err)
	}

	if conf.PORT != "8000" {
		t.Fatalf("default PORT = %q", conf.PORT)
	}
	if conf.DATABASE_URL != "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable" {
		t.Fatalf("default DATABASE_URL = %q", conf.DATABASE_URL)
	}
	if conf.OIDCType != "oidc" {
		t.Fatalf("default OIDCType = %q", conf.OIDCType)
	}
	if conf.OIDCProvider != "casdoor" {
		t.Fatalf("default OIDCProvider = %q", conf.OIDCProvider)
	}
	if conf.OIDCScopes != "openid profile email" {
		t.Fatalf("default OIDCScopes = %q", conf.OIDCScopes)
	}
	if conf.MachineAuthGrantType != "client_credentials" {
		t.Fatalf("default MachineAuthGrantType = %q", conf.MachineAuthGrantType)
	}
	if conf.AUTH_DISABLED != "false" {
		t.Fatalf("default AUTH_DISABLED = %q", conf.AUTH_DISABLED)
	}

	custom := Conf{
		OIDCIssuer:        "issuer",
		OIDCClientID:      "client-id",
		OIDCTokenEndpoint: "token-endpoint",
		OIDCClientSecret:  "client-secret",
		JWTAudience:       "jwt-audience",
	}
	if custom.OIDCIssuer != "issuer" || custom.OIDCClientID != "client-id" || custom.OIDCTokenEndpoint != "token-endpoint" || custom.OIDCClientSecret != "client-secret" || custom.JWTAudience != "jwt-audience" {
		t.Fatalf("unexpected custom conf values: %+v", custom)
	}
}
