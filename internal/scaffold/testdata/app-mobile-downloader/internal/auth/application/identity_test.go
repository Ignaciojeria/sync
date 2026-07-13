package auth

import (
	"context"
	"testing"
	"time"

	"fixtests1/internal/shared/configuration"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type fakeKeyfunc struct {
	key any
	err error
}

func (f fakeKeyfunc) Keyfunc(token *jwt.Token) (any, error)         { return f.key, f.err }
func (f fakeKeyfunc) KeyfuncCtx(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) { return f.key, f.err }
}
func (f fakeKeyfunc) Storage() jwkset.Storage { return nil }
func (f fakeKeyfunc) VerificationKeySet(ctx context.Context) (jwt.VerificationKeySet, error) {
	return jwt.VerificationKeySet{}, nil
}

func signToken(t *testing.T, secret []byte, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return s
}

func TestIdentityFromTokens(t *testing.T) {
	secret := []byte("secret-for-test")
	now := time.Now().Unix()

	conf := configuration.Conf{
		OIDCIssuer:      "https://issuer.example",
		JWTAudience:     "audience-x",
		OIDCClientID:    "client-id-fallback",
	}

	t.Run("returns identity from id_token", func(t *testing.T) {
		id := "user-1"
		name := "Alice"
		email := "alice@example.com"
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss":   conf.OIDCIssuer,
			"aud":   conf.JWTAudience,
			"sub":   id,
			"name":  name,
			"email": email,
			"exp":   now + 600,
		})
		resp := CallbackResponse{IDToken: idTok, AccessToken: "ignored"}

		keyfunc := fakeKeyfunc{key: secret}
		got, err := IdentityFromTokens(conf, keyfunc, resp)
		if err != nil {
			t.Fatalf("IdentityFromTokens() error = %v", err)
		}
		if got.Subject != id || got.Email != email || got.DisplayName != name {
			t.Fatalf("identity = %+v", got)
		}
	})

	t.Run("falls back to access_token when id_token invalid", func(t *testing.T) {
		access := signToken(t, secret, jwt.MapClaims{
			"iss":   conf.OIDCIssuer,
			"aud":   conf.JWTAudience,
			"sub":   "user-2",
			"email": "bob@example.com",
			"exp":   now + 600,
		})
		resp := CallbackResponse{IDToken: "bad-token", AccessToken: access}

		got, err := IdentityFromTokens(conf, fakeKeyfunc{key: secret}, resp)
		if err != nil {
			t.Fatalf("IdentityFromTokens() error = %v", err)
		}
		if got.Subject != "user-2" || got.Email != "bob@example.com" {
			t.Fatalf("identity = %+v", got)
		}
	})

	t.Run("prefers display_name and name over email", func(t *testing.T) {
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss":         conf.OIDCIssuer,
			"aud":         conf.JWTAudience,
			"sub":         "user-3",
			"email":       "charlie@example.com",
			"name":        "Charlie Name",
			"display_name": "Charlie Display",
			"exp":         now + 600,
		})

		got, err := IdentityFromTokens(conf, fakeKeyfunc{key: secret}, CallbackResponse{IDToken: idTok})
		if err != nil {
			t.Fatalf("IdentityFromTokens() error = %v", err)
		}
		if got.DisplayName != "Charlie Display" {
			t.Fatalf("display_name = %q", got.DisplayName)
		}
	})

	t.Run("uses audience when jwt audience not set", func(t *testing.T) {
		c := configuration.Conf{OIDCIssuer: conf.OIDCIssuer, OIDCClientID: "client-id-fallback"}
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss":   c.OIDCIssuer,
			"aud":   "client-id-fallback",
			"sub":   "user-4",
			"email": "user-4@example.com",
			"exp":   now + 600,
		})
		got, err := IdentityFromTokens(c, fakeKeyfunc{key: secret}, CallbackResponse{IDToken: idTok})
		if err != nil {
			t.Fatalf("IdentityFromTokens() error = %v", err)
		}
		if got.Subject != "user-4" {
			t.Fatalf("subject = %q", got.Subject)
		}
	})

	t.Run("rejects token with wrong issuer", func(t *testing.T) {
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss":   "https://attacker.example",
			"aud":   conf.JWTAudience,
			"sub":   "user-5",
			"email": "evil@example.com",
			"exp":   now + 600,
		})
		if _, err := IdentityFromTokens(conf, fakeKeyfunc{key: secret}, CallbackResponse{IDToken: idTok, AccessToken: idTok}); err == nil {
			t.Fatal("expected error for invalid issuer")
		}
	})

	t.Run("returns error when subject cannot be extracted", func(t *testing.T) {
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss": conf.OIDCIssuer,
			"aud": conf.JWTAudience,
			"exp": now + 600,
		})
		if _, err := IdentityFromTokens(conf, fakeKeyfunc{key: secret}, CallbackResponse{IDToken: idTok, AccessToken: idTok}); err == nil {
			t.Fatal("expected error when subject missing")
		}
	})

	t.Run("keyfunc error returns error", func(t *testing.T) {
		idTok := signToken(t, secret, jwt.MapClaims{
			"iss": conf.OIDCIssuer,
			"aud": conf.JWTAudience,
			"sub": "user-6",
			"exp": now + 600,
		})
		keyfunc := fakeKeyfunc{err: jwt.ErrSignatureInvalid}
		_, err := IdentityFromTokens(conf, keyfunc, CallbackResponse{IDToken: idTok})
		if err == nil {
			t.Fatal("expected error when keyfunc fails")
		}
	})
}

var _ keyfunc.Keyfunc = fakeKeyfunc{}
