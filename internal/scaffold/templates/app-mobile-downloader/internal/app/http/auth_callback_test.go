package app

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmoiron/sqlx"
)

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func newMockConnection(t *testing.T) (*postgresql.Connection, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &postgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}, mock
}

func newHS256Keyfunc(t *testing.T) (keyfunc.Keyfunc, []byte, string) {
	t.Helper()
	secret := []byte("super-secret-signing-key")
	kid := "test-kid"
	raw := []byte(fmt.Sprintf(`{"keys":[{"kty":"oct","k":"%s","alg":"HS256","kid":"%s"}]}`,
		base64.RawURLEncoding.EncodeToString(secret), kid,
	))
	kf, err := keyfunc.NewJWKSetJSON(raw)
	if err != nil {
		t.Fatalf("NewJWKSetJSON() error = %v", err)
	}
	return kf, secret, kid
}

func signHS256Token(t *testing.T, secret []byte, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return signed
}

func newAuthServer(t *testing.T, conf configuration.Conf, db *postgresql.Connection, jwks keyfunc.Keyfunc) *httptest.Server {
	t.Helper()
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	authCallbackHandler(s, conf, db, jwks)
	return httptest.NewServer(fs.Mux)
}

func TestExchangeAuthorizationCode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm() error = %v", err)
			}
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "abc" {
				t.Fatalf("unexpected form: %v", r.Form)
			}
			_, _ = io.WriteString(w, `{"access_token":"a","refresh_token":"r","id_token":"i","expires_in":60}`)
		}))
		defer tokenServer.Close()

		conf := configuration.Conf{OIDCTokenEndpoint: tokenServer.URL, OIDCClientID: "cid", OIDCClientSecret: "sec", OIDCRedirectURI: "https://app/cb"}
		got, err := exchangeAuthorizationCode(conf, "abc")
		if err != nil {
			t.Fatalf("exchangeAuthorizationCode() error = %v", err)
		}
		if got.AccessToken != "a" || got.IDToken != "i" || got.ExpiresIn != 60 {
			t.Fatalf("unexpected response: %+v", got)
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer tokenServer.Close()
		_, err := exchangeAuthorizationCode(configuration.Conf{OIDCTokenEndpoint: tokenServer.URL}, "abc")
		if err == nil || !strings.Contains(err.Error(), "token exchange failed") {
			t.Fatalf("expected token exchange failed error, got %v", err)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "{")
		}))
		defer tokenServer.Close()
		_, err := exchangeAuthorizationCode(configuration.Conf{OIDCTokenEndpoint: tokenServer.URL}, "abc")
		if err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("network error", func(t *testing.T) {
		_, err := exchangeAuthorizationCode(configuration.Conf{OIDCTokenEndpoint: "http://127.0.0.1:1"}, "abc")
		if err == nil {
			t.Fatal("expected network error")
		}
	})
}

func TestExtractIdentityFromTokens(t *testing.T) {
	jwks, secret, kid := newHS256Keyfunc(t)
	conf := configuration.Conf{OIDCIssuer: "issuer", OIDCClientID: "client-id"}

	t.Run("success from id token", func(t *testing.T) {
		id := signHS256Token(t, secret, kid, jwt.MapClaims{
			"iss":   "issuer",
			"aud":   "client-id",
			"sub":   "user-1",
			"email": "ignaciovl.j@gmail.com",
			"name":  "Ignacio",
		})
		identity, err := extractIdentityFromTokens(conf, jwks, authCallbackResponse{IDToken: id})
		if err != nil {
			t.Fatalf("extractIdentityFromTokens() error = %v", err)
		}
		if identity.Subject != "user-1" || identity.Email != "ignaciovl.j@gmail.com" {
			t.Fatalf("unexpected identity: %+v", identity)
		}
	})

	t.Run("falls back to access token", func(t *testing.T) {
		access := signHS256Token(t, secret, kid, jwt.MapClaims{
			"iss":   "issuer",
			"aud":   "client-id",
			"sub":   "user-2",
			"email": "ignaciovl.j@gmail.com",
		})
		identity, err := extractIdentityFromTokens(conf, jwks, authCallbackResponse{AccessToken: access})
		if err != nil {
			t.Fatalf("extractIdentityFromTokens() error = %v", err)
		}
		if identity.Subject != "user-2" {
			t.Fatalf("unexpected identity: %+v", identity)
		}
	})

	t.Run("error when subject missing", func(t *testing.T) {
		id := signHS256Token(t, secret, kid, jwt.MapClaims{
			"iss":   "issuer",
			"aud":   "client-id",
			"email": "ignaciovl.j@gmail.com",
		})
		_, err := extractIdentityFromTokens(conf, jwks, authCallbackResponse{IDToken: id})
		if err == nil {
			t.Fatal("expected subject extraction error")
		}
	})

	t.Run("error when all tokens invalid", func(t *testing.T) {
		_, err := extractIdentityFromTokens(conf, jwks, authCallbackResponse{IDToken: "bad", AccessToken: "also-bad"})
		if err == nil {
			t.Fatal("expected parse error path")
		}
	})
}

func TestPersistUserSession(t *testing.T) {
	t.Run("db nil", func(t *testing.T) {
		_, err := persistUserSession(nil, oidcIdentity{Subject: "sub"}, authCallbackResponse{})
		if err == nil || !strings.Contains(err.Error(), "db connection is nil") {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty subject", func(t *testing.T) {
		db, _ := newMockConnection(t)
		_, err := persistUserSession(db, oidcIdentity{}, authCallbackResponse{})
		if err == nil || !strings.Contains(err.Error(), "oidc subject is empty") {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("begin error", func(t *testing.T) {
		db, mock := newMockConnection(t)
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
		_, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{})
		if err == nil {
			t.Fatal("expected begin error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("user insert error", func(t *testing.T) {
		db, mock := newMockConnection(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnError(errors.New("user insert failed"))
		_, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{})
		if err == nil {
			t.Fatal("expected user insert error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("session insert error", func(t *testing.T) {
		db, mock := newMockConnection(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnError(errors.New("session insert failed"))
		_, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{ExpiresIn: 10})
		if err == nil {
			t.Fatal("expected session insert error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("commit error", func(t *testing.T) {
		db, mock := newMockConnection(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
		mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		_, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{})
		if err == nil {
			t.Fatal("expected commit error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockConnection(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
		mock.ExpectCommit()
		sessionID, err := persistUserSession(db, oidcIdentity{Subject: "sub", Email: "ignaciovl.j@gmail.com", DisplayName: "Ignacio"}, authCallbackResponse{AccessToken: "a", RefreshToken: "r", IDToken: "i", ExpiresIn: 30})
		if err != nil {
			t.Fatalf("persistUserSession() error = %v", err)
		}
		if sessionID != "s1" {
			t.Fatalf("sessionID = %q, want s1", sessionID)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})
}

func TestNullableString(t *testing.T) {
	if got := nullableString("  "); got.Valid {
		t.Fatalf("expected invalid null string, got %+v", got)
	}
	got := nullableString("  value  ")
	if !got.Valid || got.String != "value" {
		t.Fatalf("unexpected null string: %+v", got)
	}
}

func TestAuthCallbackHandlerAndLogout(t *testing.T) {
	jwks, secret, kid := newHS256Keyfunc(t)

	newTokenEndpoint := func(idToken string, status int, body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if status != 0 {
				w.WriteHeader(status)
				if body != "" {
					_, _ = io.WriteString(w, body)
				}
				return
			}
			_, _ = io.WriteString(w, fmt.Sprintf(`{"access_token":"a","refresh_token":"r","id_token":"%s","expires_in":60}`, idToken))
		}))
	}

	t.Run("missing code", func(t *testing.T) {
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback"}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		res, err := noRedirectClient().Get(srv.URL + "/auth/callback?state=s1")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("invalid oauth state", func(t *testing.T) {
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback"}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "wrong"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", res.StatusCode)
		}
	})

	t.Run("exchange error", func(t *testing.T) {
		tokenSrv := newTokenEndpoint("", http.StatusBadGateway, "upstream error")
		defer tokenSrv.Close()
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback", OIDCTokenEndpoint: tokenSrv.URL}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "s1"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", res.StatusCode)
		}
	})

	t.Run("identity extraction error", func(t *testing.T) {
		badIdentityToken := signHS256Token(t, secret, kid, jwt.MapClaims{"iss": "issuer", "aud": "client-id", "email": "ignaciovl.j@gmail.com"})
		tokenSrv := newTokenEndpoint(badIdentityToken, 0, "")
		defer tokenSrv.Close()
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback", OIDCTokenEndpoint: tokenSrv.URL, OIDCIssuer: "issuer", OIDCClientID: "client-id"}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "s1"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", res.StatusCode)
		}
	})

	t.Run("forbidden email", func(t *testing.T) {
		id := signHS256Token(t, secret, kid, jwt.MapClaims{"iss": "issuer", "aud": "client-id", "sub": "user-1", "email": "blocked@example.com"})
		tokenSrv := newTokenEndpoint(id, 0, "")
		defer tokenSrv.Close()
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback", OIDCTokenEndpoint: tokenSrv.URL, OIDCIssuer: "issuer", OIDCClientID: "client-id"}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "s1"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", res.StatusCode)
		}
	})

	t.Run("persist session error", func(t *testing.T) {
		id := signHS256Token(t, secret, kid, jwt.MapClaims{"iss": "issuer", "aud": "client-id", "sub": "user-1", "email": "ignaciovl.j@gmail.com"})
		tokenSrv := newTokenEndpoint(id, 0, "")
		defer tokenSrv.Close()
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback", OIDCTokenEndpoint: tokenSrv.URL, OIDCIssuer: "issuer", OIDCClientID: "client-id"}
		srv := newAuthServer(t, conf, nil, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "s1"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", res.StatusCode)
		}
	})

	t.Run("success and logout", func(t *testing.T) {
		id := signHS256Token(t, secret, kid, jwt.MapClaims{"iss": "issuer", "aud": "client-id", "sub": "user-1", "email": "ignaciovl.j@gmail.com", "name": "Ignacio"})
		tokenSrv := newTokenEndpoint(id, 0, "")
		defer tokenSrv.Close()
		db, mock := newMockConnection(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("sess-1"))
		mock.ExpectCommit()
		mock.ExpectExec("UPDATE sessions SET revoked_at = NOW\\(\\), updated_at = NOW\\(\\) WHERE id = \\$1").WithArgs("sess-1").WillReturnResult(sqlmock.NewResult(0, 1))

		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback", OIDCTokenEndpoint: tokenSrv.URL, OIDCIssuer: "issuer", OIDCClientID: "client-id"}
		srv := newAuthServer(t, conf, db, jwks)
		defer srv.Close()

		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/callback?state=s1&code=c1", nil)
		req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "s1"})
		res, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusFound {
			t.Fatalf("status = %d, want 302", res.StatusCode)
		}
		if got := res.Header.Get("Location"); got != postLoginRedirectPath {
			t.Fatalf("location = %q, want %q", got, postLoginRedirectPath)
		}
		cookies := res.Header.Values("Set-Cookie")
		joined := strings.Join(cookies, "\n")
		if !strings.Contains(joined, "oidc_state=") || !strings.Contains(joined, "app_session_id=sess-1") {
			t.Fatalf("unexpected cookies: %q", joined)
		}

		logoutReq, _ := http.NewRequest(http.MethodGet, srv.URL+"/auth/logout", nil)
		logoutReq.AddCookie(&http.Cookie{Name: "app_session_id", Value: "sess-1"})
		logoutRes, err := noRedirectClient().Do(logoutReq)
		if err != nil {
			t.Fatalf("logout Do() error = %v", err)
		}
		defer logoutRes.Body.Close()
		if logoutRes.StatusCode != http.StatusFound {
			t.Fatalf("logout status = %d, want 302", logoutRes.StatusCode)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("logout without session cookie", func(t *testing.T) {
		db, _ := newMockConnection(t)
		conf := configuration.Conf{OIDCRedirectURI: "https://app/callback"}
		srv := newAuthServer(t, conf, db, jwks)
		defer srv.Close()
		res, err := noRedirectClient().Get(srv.URL + "/auth/logout")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusFound {
			t.Fatalf("status = %d, want 302", res.StatusCode)
		}
	})
}

func TestPersistUserSessionNoExpiryPath(t *testing.T) {
	db, mock := newMockConnection(t)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectQuery("INSERT INTO sessions").WithArgs("u1", sql.NullString{String: "a", Valid: true}, sql.NullString{String: "", Valid: false}, sql.NullString{String: "", Valid: false}, nil).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s2"))
	mock.ExpectCommit()

	sessionID, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{AccessToken: "a", ExpiresIn: 0})
	if err != nil {
		t.Fatalf("persistUserSession() error = %v", err)
	}
	if sessionID != "s2" {
		t.Fatalf("sessionID = %q, want s2", sessionID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestExtractIdentityUsesNameFallback(t *testing.T) {
	jwks, secret, kid := newHS256Keyfunc(t)
	conf := configuration.Conf{OIDCIssuer: "issuer", OIDCClientID: "client-id"}
	token := signHS256Token(t, secret, kid, jwt.MapClaims{
		"iss":  "issuer",
		"aud":  "client-id",
		"sub":  "subject-1",
		"name": "fallback@example.com",
	})
	identity, err := extractIdentityFromTokens(conf, jwks, authCallbackResponse{IDToken: token})
	if err != nil {
		t.Fatalf("extractIdentityFromTokens() error = %v", err)
	}
	if identity.Email != "fallback@example.com" || identity.DisplayName != "fallback@example.com" {
		t.Fatalf("unexpected fallback identity: %+v", identity)
	}
}

func TestPersistUserSessionExpiryIsInFuture(t *testing.T) {
	db, mock := newMockConnection(t)
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s3"))
	mock.ExpectCommit()
	_, err := persistUserSession(db, oidcIdentity{Subject: "sub"}, authCallbackResponse{ExpiresIn: 1})
	if err != nil {
		t.Fatalf("persistUserSession() error = %v", err)
	}
}

func TestAuthCallbackHandlerUsesHTTPSExtension(t *testing.T) {
	jwks, _, _ := newHS256Keyfunc(t)
	conf := configuration.Conf{OIDCRedirectURI: "https://app.example/callback"}
	srv := newAuthServer(t, conf, nil, jwks)
	defer srv.Close()
	res, err := noRedirectClient().Get(srv.URL + "/auth/logout")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer res.Body.Close()
	cookies := strings.Join(res.Header.Values("Set-Cookie"), "\n")
	if !strings.Contains(strings.ToLower(cookies), "secure") {
		t.Fatalf("expected secure logout cookie when redirect uri is https, got %q", cookies)
	}
}
