package auth

import (
	"context"
	authapp "fixtests1/internal/auth/application"
	"fixtests1/internal/auth/infrastructure/postgresql"
	"fixtests1/internal/shared/configuration"
	sharedpostgresql "fixtests1/internal/shared/infrastructure/postgresql"
	"fixtests1/internal/shared/server"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"
)

// inMemoryKeyfunc is a keyfunc.Keyfunc backed by a single symmetric secret.
// Useful for testing the OIDC callback flow without spinning a JWKS server.
type inMemoryKeyfunc struct {
	secret []byte
}

func (k inMemoryKeyfunc) Keyfunc(token *jwt.Token) (any, error) { return k.secret, nil }
func (k inMemoryKeyfunc) KeyfuncCtx(ctx context.Context) jwt.Keyfunc {
	return func(*jwt.Token) (any, error) { return k.secret, nil }
}
func (k inMemoryKeyfunc) Storage() jwkset.Storage { return nil }
func (k inMemoryKeyfunc) VerificationKeySet(ctx context.Context) (jwt.VerificationKeySet, error) {
	return jwt.VerificationKeySet{}, nil
}

var _ keyfunc.Keyfunc = inMemoryKeyfunc{}

func setupCallbackTestServer(t *testing.T, conf configuration.Conf, store *postgresql.SessionRepository, jwks keyfunc.Keyfunc) *httptest.Server {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// If a store wasn't supplied, build a real but unused one; the test will
	// inject expectations on this mock.
	if store == nil {
		store = postgresql.NewSessionRepository(&sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")})
	} else {
		// Reuse the supplied store; create a parallel sqlmock to assert queries.
		_ = mock
	}

	fs := server.NewServer()
	s := fs
	Register(s, conf, store, jwks)
	ts := httptest.NewServer(fs.Mux)
	t.Cleanup(ts.Close)
	return ts
}

var stateOnce sync.Once

func newOauthState(t *testing.T) string {
	t.Helper()
	stateOnce.Do(func() {
		// no-op, just a convenient singleton set tracker
	})
	s, err := authapp.NewRandomState()
	if err != nil {
		t.Fatalf("NewRandomState(): %v", err)
	}
	return s
}

func TestRegisterCallbackSuccess(t *testing.T) {
	// 1. Stand up a fake OIDC token endpoint.
	idClaims := jwt.MapClaims{
		"iss":   "https://issuer.example",
		"aud":   "client-abc",
		"sub":   "user-1",
		"email": "dev@example.com",
		"exp":   jwt.NewNumericDate(time.Now().Add(600 * 1e9)).Unix(),
	}
	idToken := signHS256(t, []byte("secret"), idClaims)

	tokensrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","id_token":"` + idToken + `","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokensrv.Close()

	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client-abc",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: tokensrv.URL,
	}

	// 2. Build a sqlmock-backed store and register the test runner.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()
	store := postgresql.NewSessionRepository(&sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO users")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u-1"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO sessions")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s-1"))
	mock.ExpectCommit()

	fs := server.NewServer()
	s := fs
	Register(s, conf, store, inMemoryKeyfunc{secret: []byte("secret")})
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	state := newOauthState(t)
	c := ts.Client()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=the-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: state})
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/agent" {
		t.Fatalf("Location = %q, want /agent", loc)
	}

	var gotSession bool
	var cleared bool
	for _, ck := range res.Cookies() {
		switch ck.Name {
		case "app_session_id":
			if ck.Value == "s-1" {
				gotSession = true
			}
		case "oidc_state":
			if ck.MaxAge < 0 {
				cleared = true
			}
		}
	}
	if !gotSession {
		t.Fatal("app_session_id cookie missing or wrong value")
	}
	if !cleared {
		t.Fatal("oidc_state cookie was not cleared")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRegisterCallbackUsesMountedReturnTo(t *testing.T) {
	idClaims := jwt.MapClaims{
		"iss":   "https://issuer.example",
		"aud":   "client-abc",
		"sub":   "user-1",
		"email": "dev@example.com",
		"exp":   jwt.NewNumericDate(time.Now().Add(600 * 1e9)).Unix(),
	}
	idToken := signHS256(t, []byte("secret"), idClaims)

	tokensrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","id_token":"` + idToken + `","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokensrv.Close()

	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client-abc",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: tokensrv.URL,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New(): %v", err)
	}
	defer db.Close()
	store := postgresql.NewSessionRepository(&sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")})

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO users")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u-1"))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO sessions")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s-1"))
	mock.ExpectCommit()

	fs := server.NewServer()
	s := fs
	Register(s, conf, store, inMemoryKeyfunc{secret: []byte("secret")})
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	state := newOauthState(t)
	c := ts.Client()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=the-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: state})
	req.AddCookie(&http.Cookie{Name: "app_return_to", Value: "/agent/sessions/s-1/preview/report/tests"})
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/agent/sessions/s-1/preview/report/tests" {
		t.Fatalf("Location = %q", loc)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestRegisterCallbackBadToken(t *testing.T) {
	tokensrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"garbage","id_token":"alsogarbage"}`))
	}))
	defer tokensrv.Close()

	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client-abc",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: tokensrv.URL,
	}
	ts := setupCallbackTestServer(t, conf, nil, inMemoryKeyfunc{secret: []byte("secret")})

	state := newOauthState(t)
	c := ts.Client()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=bad-code&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: state})
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", res.StatusCode)
	}
}

func TestRegisterCallbackTokenEndpointDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: deadURL,
	}
	ts := setupCallbackTestServer(t, conf, nil, inMemoryKeyfunc{secret: []byte("secret")})

	state := newOauthState(t)
	c := ts.Client()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=x&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: state})
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when token endpoint is down", res.StatusCode)
	}
}

func TestRegisterCallbackEmailNotAllowed(t *testing.T) {
	sub := "user-1"
	idClaims := jwt.MapClaims{
		"iss":   "https://issuer.example",
		"aud":   "client",
		"sub":   sub,
		"email": "evil@attacker.example",
		"exp":   jwt.NewNumericDate(time.Now().Add(600 * 1e9)).Unix(),
	}
	idToken := signHS256(t, []byte("secret"), idClaims)

	tokensrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"at","id_token":"` + idToken + `"}`))
	}))
	defer tokensrv.Close()

	conf := configuration.Conf{
		OIDCIssuer:        "https://issuer.example",
		OIDCClientID:      "client",
		OIDCRedirectURI:   "https://app.example/cb",
		OIDCTokenEndpoint: tokensrv.URL,
	}
	ts := setupCallbackTestServer(t, conf, nil, inMemoryKeyfunc{secret: []byte("secret")})

	state := newOauthState(t)
	c := ts.Client()
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/callback?code=x&state="+state, nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: state})
	res, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for denied email", res.StatusCode)
	}
}

func signHS256(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString(): %v", err)
	}
	return signed
}
