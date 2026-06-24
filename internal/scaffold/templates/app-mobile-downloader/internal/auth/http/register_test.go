package auth

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authapp "app-mobile-downloader/internal/auth/application"
	authpostgresql "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/configuration"
	sharedpostgresql "app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-fuego/fuego"
	"github.com/jmoiron/sqlx"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var _ = sql.ErrNoRows

type fakeJWKS struct{}

var _ keyfunc.Keyfunc = fakeJWKS{}

func (fakeJWKS) Keyfunc(token *jwt.Token) (any, error)         { return nil, nil }
func (fakeJWKS) KeyfuncCtx(ctx context.Context) jwt.Keyfunc     { return nil }
func (fakeJWKS) Storage() jwkset.Storage                        { return nil }
func (fakeJWKS) VerificationKeySet(ctx context.Context) (jwt.VerificationKeySet, error) {
	return jwt.VerificationKeySet{}, nil
}

func newAuthTestServer(t *testing.T, conf configuration.Conf) (*httptest.Server, *authpostgresql.SessionRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := authpostgresql.NewSessionRepository(&sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")})

	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	Register(s, conf, store, fakeJWKS{})
	ts := httptest.NewServer(fs.Mux)
	t.Cleanup(ts.Close)
	return ts, store, mock
}

func TestRegisterLoginPage(t *testing.T) {
	conf := configuration.Conf{
		OIDCIssuer:                "https://issuer.example",
		OIDCRedirectURI:           "https://app.example/cb",
		OIDCTokenEndpoint:         "https://issuer.example/token",
		OIDCAuthorizationEndpoint: "https://issuer.example/auth",
		OIDCClientID:              "client-abc",
		JWTAudience:               "audience-x",
	}
	ts, _, _ := newAuthTestServer(t, conf)

	res, err := http.Get(ts.URL + "/auth/login")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestRegisterLoginGoogle(t *testing.T) {
	conf := configuration.Conf{
		OIDCIssuer:                "https://issuer.example",
		OIDCRedirectURI:           "https://app.example/cb",
		OIDCTokenEndpoint:         "https://issuer.example/token",
		OIDCAuthorizationEndpoint: "https://issuer.example/auth",
		OIDCLoginURL:              "https://issuer.example/login",
		OIDCClientID:              "built-in-app-mobile-downloader-client",
		PROJECT_NAME:              "mobile-downloader",
	}

	ts, _, mock := newAuthTestServer(t, conf)
	_ = mock

	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	res, err := client.Get(ts.URL + "/auth/login/google")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.HasPrefix(loc, "https://issuer.example/login?") && !strings.HasPrefix(loc, "https://accounts.google.com/") {
		t.Fatalf("unexpected redirect target %q", loc)
	}
	for _, c := range res.Cookies() {
		if c.Name == "oidc_state" && c.Value == "" {
			t.Fatal("expected non-empty oidc_state cookie")
		}
	}

	t.Run("uses google direct redirect when configured", func(t *testing.T) {
		c := conf
		c.OIDCUpstreamGoogleClientID = "google-client"
		ts2, _, _ := newAuthTestServer(t, c)
		client2 := ts2.Client()
		client2.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		res, err := client2.Get(ts2.URL + "/auth/login/google")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()

		if loc := res.Header.Get("Location"); !strings.HasPrefix(loc, "https://accounts.google.com/") {
			t.Fatalf("expected google redirect, got %q", loc)
		}
	})

	t.Run("returns 500 when login url cannot be parsed", func(t *testing.T) {
		c := configuration.Conf{
			OIDCAuthorizationEndpoint: "",
			OIDCLoginURL:              "://bad-url",
			OIDCGoogleLoginURL:        "",
		}
		ts2, _, _ := newAuthTestServer(t, c)
		res, err := http.Get(ts2.URL + "/auth/login/google")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 when login url cannot be parsed", res.StatusCode)
		}
	})

	t.Run("direct google failures return 502", func(t *testing.T) {
		c := conf
		c.OIDCUpstreamGoogleClientID = "google-client"
		c.OIDCIssuer = ""
		ts2, _, _ := newAuthTestServer(t, c)
		res, err := http.Get(ts2.URL + "/auth/login/google")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", res.StatusCode)
		}
	})
}

func TestRegisterLogout(t *testing.T) {
	conf := configuration.Conf{OIDCRedirectURI: "https://app.example/cb", OIDCClientID: "client"}
	ts, _, mock := newAuthTestServer(t, conf)
	mock.ExpectExec("UPDATE sessions SET revoked_at").
		WithArgs("sid-xyz").
		WillReturnResult(sqlmock.NewResult(0, 1))

	client := ts.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "app_session_id", Value: "sid-xyz"})
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/" {
		t.Fatalf("Location = %q, want /", loc)
	}

	cleared := false
	for _, c := range res.Cookies() {
		if c.Name == "app_session_id" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("expected app_session_id to be cleared")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}

	t.Run("logout without cookie still redirects", func(t *testing.T) {
		ts2, _, _ := newAuthTestServer(t, conf)
		client2 := ts2.Client()
		client2.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		res, err := client2.Get(ts2.URL + "/auth/logout")
		if err != nil {
			t.Fatalf("Get(): %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusFound {
			t.Fatalf("status = %d, want 302", res.StatusCode)
		}
	})
}

func TestRegisterCallbackBadRequestOnMissingCode(t *testing.T) {
	conf := configuration.Conf{
		OIDCIssuer:                "https://issuer.example",
		OIDCClientID:              "client",
		OIDCRedirectURI:           "https://app.example/cb",
		OIDCTokenEndpoint:         "https://issuer.example/token",
		OIDCAuthorizationEndpoint: "https://issuer.example/auth",
	}
	ts, _, _ := newAuthTestServer(t, conf)

	client := ts.Client()
	res, err := client.Get(ts.URL + "/auth/callback")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when code is missing", res.StatusCode)
	}
}

func TestRegisterCallbackBadRequestOnStateCookieMismatch(t *testing.T) {
	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: "https://issuer.example/token",
	}
	ts, _, _ := newAuthTestServer(t, conf)

	client := ts.Client()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=the-code&state=querystate", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "different-state"})
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when state cookies don't match", res.StatusCode)
	}
}

// silence unused import warnings
var _ = authapp.CallbackResponse{}
