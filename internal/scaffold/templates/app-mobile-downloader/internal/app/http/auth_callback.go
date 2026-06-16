package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
	"github.com/golang-jwt/jwt/v5"
)

var _ = ioc.Register(authCallbackHandler)

const postLoginRedirectPath = "/"

type authCallbackResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

type oidcIdentity struct {
	Subject     string
	Email       string
	DisplayName string
}

func authCallbackHandler(s *server.Server, conf configuration.Conf, db *postgresql.Connection, jwks keyfunc.Keyfunc) {
	fuego.Get(s.Server, "/auth/callback", func(c fuego.ContextNoBody) (any, error) {
		state := strings.TrimSpace(c.QueryParam("state"))
		code := strings.TrimSpace(c.QueryParam("code"))
		if code == "" {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing code"}
		}

		stateCookie, err := c.Request().Cookie("oidc_state")
		if err != nil || strings.TrimSpace(stateCookie.Value) == "" || stateCookie.Value != state {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: "invalid oauth state"}
		}

		resp, err := exchangeAuthorizationCode(conf, code)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		identity, err := extractIdentityFromTokens(conf, jwks, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		if !access.IsAllowedAnyEmail(identity.Email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "email sin acceso autorizado al sistema"}
		}
		sessionID, err := persistUserSession(db, identity, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "app_session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(c.Response(), c.Request(), postLoginRedirectPath, http.StatusFound)
		return nil, nil
	})

	fuego.Get(s.Server, "/auth/logout", func(c fuego.ContextNoBody) (any, error) {
		if cookie, err := c.Request().Cookie("app_session_id"); err == nil && strings.TrimSpace(cookie.Value) != "" {
			_, _ = db.Exec("UPDATE sessions SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1", strings.TrimSpace(cookie.Value))
		}
		http.SetCookie(c.Response(), &http.Cookie{Name: "app_session_id", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: isHTTPS(conf.OIDCRedirectURI), SameSite: http.SameSiteLaxMode})
		http.Redirect(c.Response(), c.Request(), "/", http.StatusFound)
		return nil, nil
	})
}

func exchangeAuthorizationCode(conf configuration.Conf, code string) (authCallbackResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	form.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	if secret := strings.TrimSpace(conf.OIDCClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}

	resp, err := http.PostForm(strings.TrimSpace(conf.OIDCTokenEndpoint), form)
	if err != nil {
		return authCallbackResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authCallbackResponse{}, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var out authCallbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return authCallbackResponse{}, err
	}
	return out, nil
}

func extractIdentityFromTokens(conf configuration.Conf, jwks keyfunc.Keyfunc, resp authCallbackResponse) (oidcIdentity, error) {
	issuer := strings.TrimSpace(conf.OIDCIssuer)
	audience := shared.FirstNonEmpty(strings.TrimSpace(conf.JWTAudience), strings.TrimSpace(conf.OIDCClientID))
	for _, tokenString := range []string{strings.TrimSpace(resp.IDToken), strings.TrimSpace(resp.AccessToken)} {
		if tokenString == "" {
			continue
		}
		claims := jwt.MapClaims{}
		opts := []jwt.ParserOption{}
		if issuer != "" {
			opts = append(opts, jwt.WithIssuer(issuer))
		}
		if audience != "" {
			opts = append(opts, jwt.WithAudience(audience))
		}
		token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc, opts...)
		if err != nil || !token.Valid {
			continue
		}
		identity := oidcIdentity{
			Subject:     strings.TrimSpace(shared.FirstStringClaim(claims, "sub")),
			Email:       strings.TrimSpace(shared.FirstStringClaim(claims, "email", "name")),
			DisplayName: strings.TrimSpace(shared.FirstStringClaim(claims, "display_name", "name", "email")),
		}
		if identity.Subject != "" {
			return identity, nil
		}
	}
	return oidcIdentity{}, fmt.Errorf("could not extract authenticated subject from oidc tokens")
}

func persistUserSession(db *postgresql.Connection, identity oidcIdentity, resp authCallbackResponse) (string, error) {
	if db == nil {
		return "", fmt.Errorf("db connection is nil")
	}
	if strings.TrimSpace(identity.Subject) == "" {
		return "", fmt.Errorf("oidc subject is empty")
	}

	tx, err := db.Beginx()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var userID string
	userSQL := "INSERT INTO users (subject, email, display_name, created_at, updated_at) " +
		"VALUES ($1, $2, $3, NOW(), NOW()) " +
		"ON CONFLICT (subject) DO UPDATE SET email = EXCLUDED.email, display_name = EXCLUDED.display_name, updated_at = NOW() " +
		"RETURNING id"
	if err := tx.Get(&userID, userSQL, identity.Subject, nullableString(identity.Email), nullableString(identity.DisplayName)); err != nil {
		return "", err
	}

	var sessionID string
	var expiresAt any
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	sessionSQL := "INSERT INTO sessions (user_id, access_token, refresh_token, id_token, expires_at, created_at, updated_at) " +
		"VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id"
	if err := tx.Get(&sessionID, sessionSQL, userID, nullableString(resp.AccessToken), nullableString(resp.RefreshToken), nullableString(resp.IDToken), expiresAt); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return sessionID, nil
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
