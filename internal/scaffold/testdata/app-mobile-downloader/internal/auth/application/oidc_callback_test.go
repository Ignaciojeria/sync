package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"app-mobile-downloader/internal/shared/configuration"
)

func TestExchangeAuthorizationCode(t *testing.T) {
	t.Run("success posts form and parses response", func(t *testing.T) {
		var gotForm url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			gotForm = r.PostForm
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"id_token":      "id-token",
				"token_type":    "Bearer",
				"expires_in":    3600,
			})
		}))
		defer srv.Close()

		conf := configuration.Conf{
			OIDCTokenEndpoint: srv.URL,
			OIDCClientID:      "client-abc",
			OIDCClientSecret:  "super-secret",
			OIDCRedirectURI:   "https://app.example/cb",
		}

		resp, err := ExchangeAuthorizationCode(conf, "the-code")
		if err != nil {
			t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
		}
		if resp.AccessToken != "access-token" || resp.RefreshToken != "refresh-token" || resp.IDToken != "id-token" || resp.TokenType != "Bearer" || resp.ExpiresIn != 3600 {
			t.Fatalf("unexpected resp: %+v", resp)
		}
		if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "the-code" {
			t.Fatalf("form = %v", gotForm)
		}
		if gotForm.Get("redirect_uri") != "https://app.example/cb" || gotForm.Get("client_id") != "client-abc" {
			t.Fatalf("missing oauth form fields: %v", gotForm)
		}
		if gotForm.Get("client_secret") != "super-secret" {
			t.Fatalf("expected client_secret to be set when configured")
		}
	})

	t.Run("omits empty client secret", func(t *testing.T) {
		var gotForm url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			gotForm = r.PostForm
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"access_token":"a"}`)
		}))
		defer srv.Close()

		conf := configuration.Conf{OIDCTokenEndpoint: srv.URL, OIDCClientID: "client"}
		if _, err := ExchangeAuthorizationCode(conf, "code"); err != nil {
			t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
		}
		if _, ok := gotForm["client_secret"]; ok {
			t.Fatalf("expected no client_secret in form, got %v", gotForm)
		}
	})

	t.Run("non-2xx response returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, "invalid_grant")
		}))
		defer srv.Close()

		conf := configuration.Conf{OIDCTokenEndpoint: srv.URL, OIDCClientID: "client"}
		_, err := ExchangeAuthorizationCode(conf, "code")
		if err == nil {
			t.Fatal("expected error from non-2xx response")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Fatalf("expected error to mention status, got %v", err)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-json")
		}))
		defer srv.Close()

		conf := configuration.Conf{OIDCTokenEndpoint: srv.URL, OIDCClientID: "client"}
		if _, err := ExchangeAuthorizationCode(conf, "code"); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("transport error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		srv.Close()

		conf := configuration.Conf{OIDCTokenEndpoint: srv.URL, OIDCClientID: "client"}
		if _, err := ExchangeAuthorizationCode(conf, "code"); err == nil {
			t.Fatal("expected transport error")
		}
	})
}
