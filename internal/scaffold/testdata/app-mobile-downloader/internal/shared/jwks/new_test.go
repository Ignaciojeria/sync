package jwks

import (
	"testing"

	"app-mobile-downloader/internal/shared/configuration"
)

func TestNewReturnsErrorWhenNoJWKSURLIsConfigured(t *testing.T) {
	_, err := New(configuration.Conf{})
	if err == nil {
		t.Fatal("expected error when no JWKS urls are configured")
	}
}

func TestNewUsesFallbackOIDCJWKSURI(t *testing.T) {
	kf, err := New(configuration.Conf{OIDCJWKSURI: "http://127.0.0.1:0/jwks.json"})
	if err != nil {
		t.Fatalf("unexpected error using fallback OIDCJWKSURI: %v", err)
	}
	if kf == nil {
		t.Fatal("expected non-nil keyfunc when fallback OIDCJWKSURI is provided")
	}
}
