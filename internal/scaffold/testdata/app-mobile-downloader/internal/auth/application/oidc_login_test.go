package auth

import (
	"net/url"
	"strings"
	"testing"

	"gitinittest5/internal/shared/configuration"
)

func TestBuildLoginURL(t *testing.T) {
	conf := configuration.Conf{
		OIDCLoginURL:               "https://issuer.example/login",
		OIDCAuthorizationEndpoint:  "https://issuer.example/authorize",
		OIDCGoogleLoginURL:         "https://issuer.example/google",
		OIDCClientID:               "client-123",
		OIDCRedirectURI:            "https://app.example/cb",
		OIDCScopes:                 "openid profile",
	}

	t.Run("uses primary login url", func(t *testing.T) {
		got, err := BuildLoginURL(conf, "state-xyz", false)
		if err != nil {
			t.Fatalf("BuildLoginURL() error = %v", err)
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("parse() error = %v", err)
		}
		if u.Scheme != "https" || u.Host != "issuer.example" || u.Path != "/login" {
			t.Fatalf("base url = %q", got)
		}
		q := u.Query()
		if q.Get("client_id") != "client-123" || q.Get("state") != "state-xyz" || q.Get("response_type") != "code" {
			t.Fatalf("unexpected query: %v", q)
		}
		if q.Get("scope") != "openid profile" {
			t.Fatalf("scope = %q", q.Get("scope"))
		}
	})

	t.Run("prefer-google uses google login url", func(t *testing.T) {
		got, err := BuildLoginURL(conf, "state", true)
		if err != nil {
			t.Fatalf("BuildLoginURL() error = %v", err)
		}
		u, _ := url.Parse(got)
		if u.Path != "/google" {
			t.Fatalf("path = %q", u.Path)
		}
	})

	t.Run("falls back to authorization endpoint", func(t *testing.T) {
		c := conf
		c.OIDCLoginURL = ""
		c.OIDCGoogleLoginURL = ""
		got, err := BuildLoginURL(c, "s", false)
		if err != nil {
			t.Fatalf("BuildLoginURL() error = %v", err)
		}
		u, _ := url.Parse(got)
		if u.Path != "/authorize" {
			t.Fatalf("path = %q", u.Path)
		}
	})

	t.Run("default scope", func(t *testing.T) {
		c := conf
		c.OIDCScopes = ""
		got, err := BuildLoginURL(c, "s", false)
		if err != nil {
			t.Fatalf("BuildLoginURL() error = %v", err)
		}
		if !strings.Contains(got, "scope=openid+profile+email") {
			t.Fatalf("expected default scope in %q", got)
		}
	})

	t.Run("invalid base url returns error", func(t *testing.T) {
		c := conf
		c.OIDCLoginURL = "://bad-url"
		if _, err := BuildLoginURL(c, "s", false); err == nil {
			t.Fatal("expected error for invalid base url")
		}
	})
}

func TestBuildDirectGoogleLoginURL(t *testing.T) {
	// ponytail: clientID `"built-in-gitinittest5-client"` codifica el slug
	// `gitinittest5` (la función busca `-<slug>-` adentro del clientID).
	// Usar un slug distinto vuelve el derive imposible y contamina el
	// resto de los casos aunque la aserción no toque el appName.
	conf := configuration.Conf{
		OIDCIssuer:                 "https://issuer.example/",
		OIDCUpstreamGoogleClientID: "google-client",
		OIDCClientID:               "built-in-gitinittest5-client",
		PROJECT_NAME:               "gitinittest5",
		OIDCRedirectURI:            "https://app.example/cb",
		OIDCScopes:                 "openid profile",
	}

	t.Run("builds packed state and google redirect", func(t *testing.T) {
		got, err := BuildDirectGoogleLoginURL(conf, "rawstate")
		if err != nil {
			t.Fatalf("BuildDirectGoogleLoginURL() error = %v", err)
		}
		if !strings.HasPrefix(got, "https://accounts.google.com/signin/oauth?") {
			t.Fatalf("expected google auth base, got %q", got)
		}
		u, _ := url.Parse(got)
		if u.Query().Get("client_id") != "google-client" {
			t.Fatalf("client_id = %q", u.Query().Get("client_id"))
		}
		if u.Query().Get("redirect_uri") != "https://issuer.example/callback" {
			t.Fatalf("redirect_uri = %q", u.Query().Get("redirect_uri"))
		}
	})

	t.Run("missing google client id", func(t *testing.T) {
		c := conf
		c.OIDCUpstreamGoogleClientID = ""
		if _, err := BuildDirectGoogleLoginURL(c, "s"); err == nil {
			t.Fatal("expected error for missing google client id")
		}
	})

	t.Run("missing issuer", func(t *testing.T) {
		c := conf
		c.OIDCIssuer = ""
		if _, err := BuildDirectGoogleLoginURL(c, "s"); err == nil {
			t.Fatal("expected error for missing issuer")
		}
	})

	t.Run("rewrites issuer with trailing slash", func(t *testing.T) {
		c := conf
		c.OIDCIssuer = "https://issuer.example/"
		got, _ := BuildDirectGoogleLoginURL(c, "s")
		u, _ := url.Parse(got)
		if u.Query().Get("redirect_uri") != "https://issuer.example/callback" {
			t.Fatalf("redirect_uri = %q", u.Query().Get("redirect_uri"))
		}
	})

	t.Run("invalid client id for project slug", func(t *testing.T) {
		c := conf
		c.OIDCClientID = "totally-different"
		if _, err := BuildDirectGoogleLoginURL(c, "s"); err == nil {
			t.Fatal("expected error when client id does not include project slug")
		}
	})

	t.Run("default scopes", func(t *testing.T) {
		c := conf
		c.OIDCScopes = ""
		got, err := BuildDirectGoogleLoginURL(c, "s")
		if err != nil {
			t.Fatalf("BuildDirectGoogleLoginURL() error = %v", err)
		}
		// Validates encoding succeeded; nothing else to assert on defaults.
		if got == "" {
			t.Fatal("expected non-empty url")
		}
	})
}

func TestDeriveCasdoorAppName(t *testing.T) {
	t.Run("extracts prefix before -<slug>-", func(t *testing.T) {
		// La función recorta hasta el final del slug incluido (excluye solo el guion
		// final del needle). Documentamos el comportamiento real del código.
		got, err := DeriveCasdoorAppName("built-in-gitinittest5-client", "gitinittest5")
		if err != nil {
			t.Fatalf("DeriveCasdoorAppName() error = %v", err)
		}
		if got != "built-in-gitinittest5" {
			t.Fatalf("got %q, want built-in-gitinittest5", got)
		}
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		got, err := DeriveCasdoorAppName("  built-in-gitinittest5-client  ", "gitinittest5")
		if err != nil {
			t.Fatalf("DeriveCasdoorAppName() error = %v", err)
		}
		if got != "built-in-gitinittest5" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("empty client id", func(t *testing.T) {
		if _, err := DeriveCasdoorAppName("  ", "slug"); err == nil {
			t.Fatal("expected error for empty client id")
		}
	})

	t.Run("empty slug", func(t *testing.T) {
		if _, err := DeriveCasdoorAppName("client", " "); err == nil {
			t.Fatal("expected error for empty slug")
		}
	})

	t.Run("slug not found in client id", func(t *testing.T) {
		if _, err := DeriveCasdoorAppName("no-match", "slug"); err == nil {
			t.Fatal("expected error when slug not found")
		}
	})
}

func TestIsHTTPS(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"https://example.com", true},
		{"HTTPS://example.com", true},
		{"http://example.com", false},
		{"notaurl", false},
		{"   ", false},
	}
	for _, c := range cases {
		if got := IsHTTPS(c.raw); got != c.want {
			t.Errorf("IsHTTPS(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}
