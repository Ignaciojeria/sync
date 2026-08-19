package agent

import (
	"encoding/base64"
	authapp "lastmile-agents/internal/auth/application"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeLookup struct {
	rec        authapp.Session
	refreshErr error
}

func (f *fakeLookup) FindActiveSessionByID(id string) (authapp.Session, error) {
	return f.rec, nil
}
func (f *fakeLookup) UpdateSessionTokens(id, at, rt, it string, exp *time.Time) error {
	f.rec.AccessToken = at
	f.rec.RefreshToken = rt
	f.rec.IDToken = it
	f.rec.ExpiresAt = exp
	return f.refreshErr
}

// makeTestJWT arma un JWT con header {"alg":"RS256"} y payload
// {"exp":<unix>}. NO es un token firmado — sólo lo necesitamos para
// probar la lógica de freshness basada en el claim exp.
func makeTestJWT(exp time.Time) string {
	header := `{"alg":"RS256","typ":"JWT"}`
	payload := `{"exp":` + itoa(int64(exp.Unix())) + `}`
	enc := func(s string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(s))
	}
	return enc(header) + "." + enc(payload) + ".sig"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestJwtExpClaim(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).Truncate(time.Second)
	past := time.Now().Add(-1 * time.Minute).Truncate(time.Second)
	if got := jwtExpClaim(makeTestJWT(future)); !got.Equal(future) {
		t.Fatalf("future jwt: got %v want %v", got, future)
	}
	if got := jwtExpClaim(makeTestJWT(past)); !got.Equal(past) {
		t.Fatalf("past jwt: got %v want %v", got, past)
	}
	if got := jwtExpClaim("not-a-jwt"); !got.IsZero() {
		t.Fatalf("invalid jwt: got %v want zero", got)
	}
	if got := jwtExpClaim(""); !got.IsZero() {
		t.Fatalf("empty jwt: got %v want zero", got)
	}
}

func TestResolveFreshIDToken_NoRefreshWhenFresh(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	fl := &fakeLookup{rec: authapp.Session{
		ID:        "s1",
		IDToken:   makeTestJWT(future),
		ExpiresAt: &future,
	}}
	rec, err := resolveFreshIDToken(t.Context(), fl, OIDCRefreshConfig{}, "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.IDToken == "" {
		t.Fatal("expected non-empty IDToken after fresh check")
	}
}

func TestResolveFreshIDToken_RefreshWhenIDTokenEmpty(t *testing.T) {
	fl := &fakeLookup{rec: authapp.Session{ID: "s1", IDToken: "", RefreshToken: ""}}
	_, err := resolveFreshIDToken(t.Context(), fl, OIDCRefreshConfig{
		TokenEndpoint: "https://i.example/token",
		ClientID:      "c1",
	}, "s1")
	if err == nil {
		t.Fatal("expected error when refresh_token empty and IDToken empty")
	}
	if !strings.Contains(err.Error(), "refresh_token") {
		t.Fatalf("expected error to mention refresh_token, got: %v", err)
	}
}

func TestResolveFreshIDToken_RefreshWhenJWTExpired(t *testing.T) {
	past := time.Now().Add(-1 * time.Minute)
	future := time.Now().Add(1 * time.Hour)
	fl := &fakeLookup{rec: authapp.Session{
		ID:           "s1",
		IDToken:      makeTestJWT(past),
		ExpiresAt:    &future, // DB dice que está vigente, pero el JWT exp mintió
		Subject:      "u",
		Email:        "u@x",
		RefreshToken: "",
	}}
	_, err := resolveFreshIDToken(t.Context(), fl, OIDCRefreshConfig{
		TokenEndpoint: "https://i.example/token",
		ClientID:      "c1",
	}, "s1")
	if err == nil {
		t.Fatal("expected error: JWT exp en el pasado debe disparar refresh")
	}
	if !strings.Contains(err.Error(), "refresh_token") {
		t.Fatalf("expected error to mention refresh_token (refresh falló), got: %v", err)
	}
}

func TestResolveFreshIDToken_RefreshWhenJWTNotParseable(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	fl := &fakeLookup{rec: authapp.Session{
		ID:           "s1",
		IDToken:      "not-a-jwt",
		ExpiresAt:    &future,
		RefreshToken: "",
	}}
	_, err := resolveFreshIDToken(t.Context(), fl, OIDCRefreshConfig{
		TokenEndpoint: "https://i.example/token",
		ClientID:      "c1",
	}, "s1")
	if err == nil {
		t.Fatal("expected error: JWT sin exp parseable debe disparar refresh")
	}
}

func TestSessionIDFromCookie(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, ok := sessionIDFromCookie(httptest.NewRequest(http.MethodGet, "/x", nil))
		if ok {
			t.Fatal("expected ok=false on missing cookie")
		}
	})
	t.Run("empty_value", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.AddCookie(&http.Cookie{Name: "app_session_id", Value: "   "})
		_, ok := sessionIDFromCookie(r)
		if ok {
			t.Fatal("expected ok=false on whitespace-only cookie value")
		}
	})
	t.Run("present", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.AddCookie(&http.Cookie{Name: "app_session_id", Value: "abc123"})
		id, ok := sessionIDFromCookie(r)
		if !ok || id != "abc123" {
			t.Fatalf("got id=%q ok=%v", id, ok)
		}
	})
}
