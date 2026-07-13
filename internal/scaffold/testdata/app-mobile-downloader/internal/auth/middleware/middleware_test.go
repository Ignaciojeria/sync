package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authapp "fixtests1/internal/auth/application"
	"fixtests1/internal/shared"
	"fixtests1/internal/shared/configuration"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
)

type fakeStore struct {
	record    authapp.Session
	findErr   error
	updateErr error
	updated   struct {
		sessionID    string
		accessToken  string
		refreshToken string
		idToken      string
		expiresAt    *time.Time
		called       bool
	}
}

func (f *fakeStore) FindActiveSessionByID(sessionID string) (authapp.Session, error) {
	return f.record, f.findErr
}

func (f *fakeStore) UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error {
	f.updated.sessionID = sessionID
	f.updated.accessToken = accessToken
	f.updated.refreshToken = refreshToken
	f.updated.idToken = idToken
	f.updated.expiresAt = expiresAt
	f.updated.called = true
	return f.updateErr
}

func (f *fakeStore) CreateUserSession(identity authapp.Identity, resp authapp.CallbackResponse) (string, error) {
	return "", nil
}

func (f *fakeStore) RevokeSession(sessionID string) error {
	return nil
}

type fakeKeyfunc struct {
	key any
	err error
}

func (f fakeKeyfunc) Keyfunc(token *jwt.Token) (any, error) { return f.key, f.err }
func (f fakeKeyfunc) KeyfuncCtx(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) { return f.key, f.err }
}
func (f fakeKeyfunc) Storage() jwkset.Storage { return nil }
func (f fakeKeyfunc) VerificationKeySet(ctx context.Context) (jwt.VerificationKeySet, error) {
	return jwt.VerificationKeySet{}, nil
}

func signedHS256Token(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return s
}

func TestJWTClaimsFromContext(t *testing.T) {
	t.Run("missing claims", func(t *testing.T) {
		if _, ok := JWTClaimsFromContext(context.Background()); ok {
			t.Fatal("expected no claims in empty context")
		}
	})

	t.Run("claims present", func(t *testing.T) {
		want := jwt.MapClaims{"sub": "user-1"}
		ctx := context.WithValue(context.Background(), claimsContextKey, want)
		got, ok := JWTClaimsFromContext(ctx)
		if !ok {
			t.Fatal("expected claims in context")
		}
		if got["sub"] != want["sub"] {
			t.Fatalf("expected subject %v, got %v", want["sub"], got["sub"])
		}
	})
}

func TestPrincipalFromClaims(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   Principal
	}{
		{
			name:   "human token keeps subject",
			claims: jwt.MapClaims{"sub": " user-1 ", "email": "a@example.com"},
			want:   Principal{Subject: "user-1", MachineClientID: "user-1", IsMachine: false},
		},
		{
			name:   "client credentials token uses client id",
			claims: jwt.MapClaims{"gty": "client-credentials", "client_id": " machine-1 "},
			want:   Principal{Subject: "", MachineClientID: "machine-1", IsMachine: true},
		},
		{
			name:   "machine token falls back to audience",
			claims: jwt.MapClaims{"token_use": "machine", "aud": []any{"", "aud-client"}},
			want:   Principal{Subject: "", MachineClientID: "aud-client", IsMachine: true},
		},
		{
			name:   "application token falls back to name when subject missing",
			claims: jwt.MapClaims{"token_use": "application", "name": "worker-app"},
			want:   Principal{Subject: "", MachineClientID: "worker-app", IsMachine: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PrincipalFromClaims(tt.claims)
			if got != tt.want {
				t.Fatalf("PrincipalFromClaims() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeGrantType(t *testing.T) {
	if got := normalizeGrantType(" Client-Credentials "); got != "client_credentials" {
		t.Fatalf("normalizeGrantType() = %q", got)
	}
}

func TestFirstAudienceClaim(t *testing.T) {
	tests := []struct {
		name   string
		claims jwt.MapClaims
		want   string
	}{
		{name: "string audience", claims: jwt.MapClaims{"aud": " api "}, want: "api"},
		{name: "string slice audience", claims: jwt.MapClaims{"aud": []string{"", "editor"}}, want: "editor"},
		{name: "any slice audience", claims: jwt.MapClaims{"aud": []any{123, "mobile"}}, want: "mobile"},
		{name: "missing audience", claims: jwt.MapClaims{}, want: ""},
		{name: "unsupported audience type", claims: jwt.MapClaims{"aud": 123}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstAudienceClaim(tt.claims); got != tt.want {
				t.Fatalf("firstAudienceClaim() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFirstStringClaim(t *testing.T) {
	claims := jwt.MapClaims{"name": "", "email": " user@example.com "}
	if got := shared.FirstStringClaim(claims, "name", "email"); got != "user@example.com" {
		t.Fatalf("shared.FirstStringClaim() = %q", got)
	}
}

func TestFirstNonEmptyHelpers(t *testing.T) {
	if got := shared.FirstNonEmpty(" ", "value"); got != "value" {
		t.Fatalf("shared.FirstNonEmpty() = %q", got)
	}
	if got := shared.FirstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("shared.FirstNonEmpty() = %q, want empty string", got)
	}
	if got := shared.FirstNonEmpty(" ", "machine-1"); got != "machine-1" {
		t.Fatalf("FirstNonEmpty() = %q", got)
	}
	if got := shared.FirstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("FirstNonEmpty() = %q, want empty string", got)
	}
}

func TestIsAuthorizedPathClaims(t *testing.T) {
	if !isAuthorizedPathClaims("/home", jwt.MapClaims{}) {
		t.Fatal("expected requests without email claim to be allowed")
	}
	if !isAuthorizedPathClaims("/home", jwt.MapClaims{"email": "dev@example.com"}) {
		t.Fatal("expected allowed app email on app path")
	}
	if isAuthorizedPathClaims("/report/tests", jwt.MapClaims{"email": "unknown@example.com"}) {
		t.Fatal("expected unknown editor email to be denied")
	}
}

func TestShouldRedirectToLogin(t *testing.T) {
	if shouldRedirectToLogin(nil) {
		t.Fatal("nil request should not redirect")
	}
	tests := []struct {
		name   string
		method string
		accept string
		want   bool
	}{
		{name: "get html", method: http.MethodGet, accept: "text/html", want: true},
		{name: "head xhtml", method: http.MethodHead, accept: "application/xhtml+xml", want: true},
		{name: "post html", method: http.MethodPost, accept: "text/html", want: false},
		{name: "json accept", method: http.MethodGet, accept: "application/json", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/private", nil)
			req.Header.Set("Accept", tt.accept)
			if got := shouldRedirectToLogin(req); got != tt.want {
				t.Fatalf("shouldRedirectToLogin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathHelpers(t *testing.T) {
	if !isEditorPath("/editor") || !isEditorPath("/assets/app.js") || !isEditorPath("/report/tests") {
		t.Fatal("expected editor paths to be recognized")
	}
	if isEditorPath("/home") {
		t.Fatal("did not expect /home to be an editor path")
	}
	if !isPublicPath("/auth/login") || !isPublicPath("/manifest.json") || !isPublicPath("/logo.svg") || !isPublicPath("/agent/sessions/s-1/preview/") || isPublicPath("/") || isPublicPath("/private") {
		t.Fatal("unexpected public path result")
	}
}

func TestWriteResponses(t *testing.T) {
	t.Run("unauthorized", func(t *testing.T) {
		rr := httptest.NewRecorder()
		writeUnauthorized(rr)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "unauthorized")
	})

	t.Run("forbidden", func(t *testing.T) {
		rr := httptest.NewRecorder()
		writeForbidden(rr)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "forbidden")
	})
}

func TestClaimsFromSessionCookie(t *testing.T) {
	t.Run("missing cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		rr := httptest.NewRecorder()
		if claims, ok := claimsFromSessionCookie(req, rr, &fakeStore{}, configuration.Conf{}); ok || claims != nil {
			t.Fatal("expected no claims without cookie")
		}
	})

	t.Run("store error clears cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		if claims, ok := claimsFromSessionCookie(req, rr, &fakeStore{findErr: errors.New("not found")}, configuration.Conf{}); ok || claims != nil {
			t.Fatal("expected no claims when store lookup fails")
		}
		if !strings.Contains(rr.Header().Get("Set-Cookie"), sessionCookieName+"=") {
			t.Fatal("expected session cookie to be cleared")
		}
	})

	t.Run("expired session without token endpoint clears cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		expired := time.Now().Add(-time.Minute)
		store := &fakeStore{record: authapp.Session{ID: "sid-1", Subject: "sub-1", RefreshToken: "refresh", ExpiresAt: &expired}}
		if claims, ok := claimsFromSessionCookie(req, rr, store, configuration.Conf{}); ok || claims != nil {
			t.Fatal("expected no claims without token endpoint for expired session")
		}
	})

	t.Run("expired session refresh failure clears cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		expired := time.Now().Add(-time.Minute)
		store := &fakeStore{record: authapp.Session{ID: "sid-1", Subject: "sub-1", RefreshToken: "refresh", ExpiresAt: &expired}}
		conf := configuration.Conf{OIDCTokenEndpoint: "http://127.0.0.1:0/token", OIDCClientID: "client"}
		if claims, ok := claimsFromSessionCookie(req, rr, store, conf); ok || claims != nil {
			t.Fatal("expected no claims when refresh fails")
		}
	})

	t.Run("success builds claims", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		store := &fakeStore{record: authapp.Session{ID: "sid-1", Subject: "sub-1", Email: " user@example.com ", DisplayName: " User "}}
		claims, ok := claimsFromSessionCookie(req, rr, store, configuration.Conf{})
		if !ok {
			t.Fatal("expected claims from valid session")
		}
		if claims["sub"] != "sub-1" || claims["sid"] != "sid-1" || claims["email"] != "user@example.com" || claims["name"] != "User" {
			t.Fatalf("unexpected claims: %#v", claims)
		}
	})
}

func TestRefreshSessionTokens(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		store := &fakeStore{}
		conf := configuration.Conf{OIDCTokenEndpoint: "http://127.0.0.1:0/token", OIDCClientID: "client"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh"}
		if err := refreshSessionTokens(store, conf, rec); err == nil {
			t.Fatal("expected http error")
		}
	})

	t.Run("non-2xx response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer ts.Close()
		store := &fakeStore{}
		conf := configuration.Conf{OIDCTokenEndpoint: ts.URL, OIDCClientID: "client"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh"}
		if err := refreshSessionTokens(store, conf, rec); err == nil {
			t.Fatal("expected non-2xx error")
		}
	})

	t.Run("empty access token", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"refresh_token": "new-refresh"})
		}))
		defer ts.Close()
		store := &fakeStore{}
		conf := configuration.Conf{OIDCTokenEndpoint: ts.URL, OIDCClientID: "client"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh"}
		if err := refreshSessionTokens(store, conf, rec); err == nil {
			t.Fatal("expected empty access token error")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{"))
		}))
		defer ts.Close()
		store := &fakeStore{}
		conf := configuration.Conf{OIDCTokenEndpoint: ts.URL, OIDCClientID: "client"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh"}
		if err := refreshSessionTokens(store, conf, rec); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("update store error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access"})
		}))
		defer ts.Close()
		store := &fakeStore{updateErr: errors.New("update failed")}
		conf := configuration.Conf{OIDCTokenEndpoint: ts.URL, OIDCClientID: "client"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh"}
		if err := refreshSessionTokens(store, conf, rec); err == nil {
			t.Fatal("expected update error")
		}
	})

	t.Run("success updates store and record", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "expires_in": 60.0})
		}))
		defer ts.Close()
		store := &fakeStore{}
		conf := configuration.Conf{OIDCTokenEndpoint: ts.URL, OIDCClientID: "client", OIDCClientSecret: "secret"}
		rec := &authapp.Session{ID: "sid-1", RefreshToken: "refresh", IDToken: "old-id"}
		if err := refreshSessionTokens(store, conf, rec); err != nil {
			t.Fatalf("refreshSessionTokens() error = %v", err)
		}
		if !store.updated.called || store.updated.sessionID != "sid-1" || store.updated.accessToken != "new-access" || store.updated.refreshToken != "refresh" || store.updated.idToken != "old-id" || store.updated.expiresAt == nil {
			t.Fatalf("unexpected update payload: %+v", store.updated)
		}
		if rec.AccessToken != "new-access" || rec.ExpiresAt == nil {
			t.Fatalf("unexpected updated record: %+v", rec)
		}
	})
}

func TestParseJWTClaims(t *testing.T) {
	secret := []byte("secret")
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1", "iss": "issuer", "aud": "audience"})
	tokenString, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}

	claims, err := parseJWTClaims(fakeKeyfunc{key: secret}, tokenString, "issuer", "audience")
	if err != nil {
		t.Fatalf("parseJWTClaims() error = %v", err)
	}
	if claims["sub"] != "user-1" {
		t.Fatalf("unexpected claims: %#v", claims)
	}

	if _, err := parseJWTClaims(fakeKeyfunc{err: errors.New("bad keyfunc")}, tokenString, "issuer", "audience"); err == nil {
		t.Fatal("expected keyfunc error")
	}
	if _, err := parseJWTClaims(fakeKeyfunc{key: secret}, tokenString, "wrong-issuer", "audience"); err == nil {
		t.Fatal("expected issuer validation error")
	}

	t.Run("works without issuer and audience", func(t *testing.T) {
		claims, err := parseJWTClaims(fakeKeyfunc{key: secret}, tokenString, "", "")
		if err != nil {
			t.Fatalf("parseJWTClaims() error = %v", err)
		}
		if claims["sub"] != "user-1" {
			t.Fatalf("unexpected claims: %#v", claims)
		}
	})
}

func TestClearSessionCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	clearSessionCookie(rr)
	if !strings.Contains(rr.Header().Get("Set-Cookie"), sessionCookieName+"=") {
		t.Fatal("expected session cookie to be cleared")
	}
}

func TestJWTMiddleware(t *testing.T) {
	t.Run("public path bypasses auth", func(t *testing.T) {
		called := false
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/manifest.json", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !called || rr.Code != http.StatusNoContent {
			t.Fatalf("expected next handler to run, called=%v status=%d", called, rr.Code)
		}
	})

	t.Run("auth disabled injects dev claims", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		var gotClaims jwt.MapClaims
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotClaims, _ = JWTClaimsFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("X-Dev-Sub", "dev-123")
		req.Header.Set("X-Dev-Email", "dev@example.com")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rr.Code)
		}
		if gotClaims["sub"] != "dev-123" || gotClaims["email"] != "dev@example.com" {
			t.Fatalf("unexpected claims: %#v", gotClaims)
		}
	})

	t.Run("auth disabled injects default claims", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "true")
		var gotClaims jwt.MapClaims
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotClaims, _ = JWTClaimsFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rr.Code)
		}
		if gotClaims["sub"] != "dev-user" || gotClaims["email"] != "dev@example.com" {
			t.Fatalf("unexpected default claims: %#v", gotClaims)
		}
	})

	t.Run("html request redirects to login", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("status = %d", rr.Code)
		}
		if location := rr.Header().Get("Location"); location != "/auth/login" {
			t.Fatalf("location = %q", location)
		}
	})

	t.Run("mounted auth login path is treated as public", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		called := false
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s-1/preview/auth/login", nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !called {
			t.Fatal("expected public mounted auth path to reach next handler")
		}
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d", rr.Code)
		}
	})

	t.Run("html request under mount redirects to mounted login and stores return path", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s-1/preview/private?tab=1", nil)
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("status = %d", rr.Code)
		}
		if location := rr.Header().Get("Location"); location != "/agent/sessions/s-1/preview/auth/login" {
			t.Fatalf("location = %q", location)
		}
		found := false
		for _, ck := range rr.Result().Cookies() {
			if ck.Name == "app_return_to" {
				found = true
				if ck.Value != "/agent/sessions/s-1/preview/private?tab=1" {
					t.Fatalf("return_to = %q", ck.Value)
				}
			}
		}
		if !found {
			t.Fatal("expected app_return_to cookie")
		}
	})

	t.Run("session store claims unauthorized path returns forbidden", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		store := &fakeStore{record: authapp.Session{ID: "sid-1", Subject: "sub-1", Email: "unknown@example.com"}}
		handler := JWTMiddleware(nil, store, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))
		req := httptest.NewRequest(http.MethodGet, "/report/tests", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "forbidden")
	})

	t.Run("session store claims authorized path reaches next", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		store := &fakeStore{record: authapp.Session{ID: "sid-1", Subject: "sub-1", Email: "dev@example.com"}}
		called := false
		handler := JWTMiddleware(nil, store, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/report/tests", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "sid-1"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if !called || rr.Code != http.StatusNoContent {
			t.Fatalf("expected next handler to run, called=%v status=%d", called, rr.Code)
		}
	})

	t.Run("api request returns unauthorized", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		handler := JWTMiddleware(nil, nil, configuration.Conf{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Accept", "application/json")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "unauthorized")
	})

	t.Run("invalid bearer token redirects html", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		handler := JWTMiddleware(fakeKeyfunc{key: []byte("secret")}, nil, configuration.Conf{OIDCIssuer: "issuer", JWTAudience: "audience"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Authorization", "Bearer invalid.token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusFound {
			t.Fatalf("status = %d", rr.Code)
		}
		if location := rr.Header().Get("Location"); location != "/auth/login" {
			t.Fatalf("location = %q", location)
		}
	})

	t.Run("invalid bearer token returns unauthorized for api", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		handler := JWTMiddleware(fakeKeyfunc{key: []byte("secret")}, nil, configuration.Conf{OIDCIssuer: "issuer", JWTAudience: "audience"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/private", nil)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer invalid.token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "unauthorized")
	})

	t.Run("valid bearer unauthorized path returns forbidden", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		secret := []byte("secret")
		token := signedHS256Token(t, secret, jwt.MapClaims{"sub": "u1", "iss": "issuer", "aud": "audience", "email": "unknown@example.com"})
		handler := JWTMiddleware(fakeKeyfunc{key: secret}, nil, configuration.Conf{OIDCIssuer: "issuer", JWTAudience: "audience"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not be called")
		}))

		req := httptest.NewRequest(http.MethodGet, "/report/tests", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rr.Code)
		}
		assertJSONError(t, rr, "forbidden")
	})

	t.Run("valid bearer authorized reaches next", func(t *testing.T) {
		t.Setenv("AUTH_DISABLED", "false")
		secret := []byte("secret")
		token := signedHS256Token(t, secret, jwt.MapClaims{"sub": "u1", "iss": "issuer", "aud": "audience", "email": "dev@example.com"})
		called := false
		handler := JWTMiddleware(fakeKeyfunc{key: secret}, nil, configuration.Conf{OIDCIssuer: "issuer", JWTAudience: "audience"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/report/tests", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !called || rr.Code != http.StatusNoContent {
			t.Fatalf("expected next handler to run, called=%v status=%d", called, rr.Code)
		}
	})
}

func TestRequireEditor(t *testing.T) {
	t.Run("rejects when no claims present", func(t *testing.T) {
		called := false
		handler := RequireEditor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/editor", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rr.Code)
		}
		if called {
			t.Fatal("next handler should not run")
		}
		assertJSONError(t, rr, "unauthorized")
	})

	t.Run("rejects non-editor emails", func(t *testing.T) {
		claims := jwt.MapClaims{"email": "blocked@example.com"}
		ctx := context.WithValue(context.Background(), claimsContextKey, claims)

		called := false
		handler := RequireEditor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/editor", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
		if called {
			t.Fatal("next handler should not run")
		}
		assertJSONError(t, rr, "forbidden")
	})

	t.Run("allows editor email", func(t *testing.T) {
		// El paquete shared ya expone dev@example.com como editor permitido (ver internal/shared/access.go).
		claims := jwt.MapClaims{"email": "dev@example.com"}
		ctx := context.WithValue(context.Background(), claimsContextKey, claims)

		called := false
		handler := RequireEditor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		req := httptest.NewRequest(http.MethodGet, "/editor", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if !called {
			t.Fatal("next handler should run")
		}
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", rr.Code)
		}
	})

	t.Run("ignores empty email claim", func(t *testing.T) {
		claims := jwt.MapClaims{"email": "   "}
		ctx := context.WithValue(context.Background(), claimsContextKey, claims)
		handler := RequireEditor()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("next handler should not run")
		}))
		req := httptest.NewRequest(http.MethodGet, "/editor", nil).WithContext(ctx)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 for empty email", rr.Code)
		}
	})
}

func assertJSONError(t *testing.T, rr *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json body: %v", err)
	}
	if body["error"] != want {
		t.Fatalf("error = %q, want %q", body["error"], want)
	}
}
