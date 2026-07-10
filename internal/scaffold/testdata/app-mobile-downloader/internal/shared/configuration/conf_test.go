package configuration

import (
	"sync"
	"testing"
)

func TestNewConf(t *testing.T) {
	once = sync.Once{}
	t.Setenv("PORT", "9090")
	t.Setenv("PROJECT_NAME", "scaffoldxd1")
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
	if conf.PROJECT_NAME != "scaffoldxd1" {
		t.Fatalf("PROJECT_NAME = %q", conf.PROJECT_NAME)
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
