package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	authapp "app-mobile-downloader/internal/auth/application"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	"app-mobile-downloader/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	claimsContextKey  contextKey = "jwt_claims"
	sessionCookieName            = "app_session_id"
)

type sessionStore interface {
	FindActiveSessionByID(sessionID string) (authapp.Session, error)
	UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error
}

func JWTMiddleware(
	jwks keyfunc.Keyfunc,
	store sessionStore,
	conf configuration.Conf,
) func(http.Handler) http.Handler {
	issuer := strings.TrimSpace(conf.OIDCIssuer)
	audience := shared.FirstNonEmpty(strings.TrimSpace(conf.JWTAudience), strings.TrimSpace(conf.OIDCClientID))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if os.Getenv("AUTH_DISABLED") == "true" {
				sub := strings.TrimSpace(r.Header.Get("X-Dev-Sub"))
				if sub == "" {
					sub = "dev-user"
				}
				email := strings.TrimSpace(r.Header.Get("X-Dev-Email"))
				if email == "" {
					email = "dev@example.com"
				}
				ctx := context.WithValue(r.Context(), claimsContextKey, jwt.MapClaims{"sub": sub, "email": email})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if store != nil {
				if claims, ok := claimsFromSessionCookie(r, w, store, conf); ok {
					if !isAuthorizedPathClaims(r.URL.Path, claims) {
						writeForbidden(w)
						return
					}
					ctx := context.WithValue(r.Context(), claimsContextKey, claims)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				if shouldRedirectToLogin(r) {
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}
				writeUnauthorized(w)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := parseJWTClaims(jwks, tokenString, issuer, audience)
			if err != nil {
				if shouldRedirectToLogin(r) {
					http.Redirect(w, r, "/auth/login", http.StatusFound)
					return
				}
				writeUnauthorized(w)
				return
			}
			if !isAuthorizedPathClaims(r.URL.Path, claims) {
				writeForbidden(w)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type Principal struct {
	Subject         string
	MachineClientID string
	IsMachine       bool
}

func JWTClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(jwt.MapClaims)
	return claims, ok
}

func PrincipalFromClaims(claims jwt.MapClaims) Principal {
	subject, _ := claims["sub"].(string)
	subject = strings.TrimSpace(subject)
	grantType := normalizeGrantType(shared.FirstStringClaim(claims, "gty", "grant_type"))
	tokenUse := strings.ToLower(strings.TrimSpace(shared.FirstStringClaim(claims, "token_use", "type")))
	isMachine := grantType == "client_credentials" || tokenUse == "machine" || tokenUse == "application"

	machineClientID := shared.FirstStringClaim(claims, "client_id", "azp", "cid")
	if strings.TrimSpace(machineClientID) == "" && isMachine {
		machineClientID = firstAudienceClaim(claims)
	}
	if strings.TrimSpace(machineClientID) == "" {
		machineClientID = firstNonEmptyMachineID(subject, shared.FirstStringClaim(claims, "name", "id"))
	}
	if subject == "" && strings.TrimSpace(machineClientID) != "" {
		isMachine = true
	}
	if tokenUse == "application" && strings.TrimSpace(machineClientID) != "" {
		isMachine = true
	}
	return Principal{
		Subject:         subject,
		MachineClientID: strings.TrimSpace(machineClientID),
		IsMachine:       isMachine,
	}
}

func claimsFromSessionCookie(r *http.Request, w http.ResponseWriter, store sessionStore, conf configuration.Conf) (jwt.MapClaims, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil, false
	}

	rec, err := store.FindActiveSessionByID(strings.TrimSpace(cookie.Value))
	if err != nil {
		clearSessionCookie(w)
		return nil, false
	}

	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		if strings.TrimSpace(conf.OIDCTokenEndpoint) == "" || strings.TrimSpace(rec.RefreshToken) == "" {
			clearSessionCookie(w)
			return nil, false
		}
		if err := refreshSessionTokens(store, conf, &rec); err != nil {
			clearSessionCookie(w)
			return nil, false
		}
	}

	claims := jwt.MapClaims{
		"sub": rec.Subject,
		"sid": rec.ID,
	}
	if strings.TrimSpace(rec.Email) != "" {
		claims["email"] = strings.TrimSpace(rec.Email)
	}
	if strings.TrimSpace(rec.DisplayName) != "" {
		claims["name"] = strings.TrimSpace(rec.DisplayName)
	}
	return claims, true
}

func refreshSessionTokens(store sessionStore, conf configuration.Conf, rec *authapp.Session) error {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(rec.RefreshToken))
	form.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	if secret := strings.TrimSpace(conf.OIDCClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}

	resp, err := http.PostForm(strings.TrimSpace(conf.OIDCTokenEndpoint), form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("refresh token exchange failed with status %d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	accessToken, _ := out["access_token"].(string)
	if strings.TrimSpace(accessToken) == "" {
		return fmt.Errorf("refresh token exchange returned empty access token")
	}
	refreshToken, _ := out["refresh_token"].(string)
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(rec.RefreshToken)
	}
	idToken, _ := out["id_token"].(string)
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		idToken = strings.TrimSpace(rec.IDToken)
	}
	var expiresAt *time.Time
	if expiresIn, ok := out["expires_in"].(float64); ok && int(expiresIn) > 0 {
		ts := time.Now().Add(time.Duration(int(expiresIn)) * time.Second)
		expiresAt = &ts
	}
	if err := store.UpdateSessionTokens(rec.ID, strings.TrimSpace(accessToken), refreshToken, idToken, expiresAt); err != nil {
		return err
	}

	rec.AccessToken = strings.TrimSpace(accessToken)
	rec.RefreshToken = refreshToken
	rec.IDToken = idToken
	rec.ExpiresAt = expiresAt
	return nil
}

func parseJWTClaims(jwks keyfunc.Keyfunc, tokenString, issuer, audience string) (jwt.MapClaims, error) {
	opts := []jwt.ParserOption{}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		opts = append(opts, jwt.WithAudience(audience))
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc, opts...)
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func normalizeGrantType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.ReplaceAll(v, "-", "_")
	return v
}

func firstNonEmptyMachineID(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstAudienceClaim(claims jwt.MapClaims) string {
	raw, ok := claims["aud"]
	if !ok || raw == nil {
		return ""
	}
	switch aud := raw.(type) {
	case string:
		return strings.TrimSpace(aud)
	case []string:
		for _, value := range aud {
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	case []any:
		for _, value := range aud {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func isAuthorizedPathClaims(path string, claims jwt.MapClaims) bool {
	email := shared.FirstStringClaim(claims, "email")
	if strings.TrimSpace(email) == "" {
		return true
	}
	if isEditorPath(path) {
		return access.IsAllowedEditorEmail(email)
	}
	return access.IsAllowedAppEmail(email)
}

func shouldRedirectToLogin(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	return strings.Contains(accept, "text/html") || strings.Contains(accept, "application/xhtml+xml")
}

func isEditorPath(path string) bool {
	path = strings.TrimSpace(path)
	switch path {
	case "/editor", "/api":
		return true
	default:
		return strings.HasPrefix(path, "/editor/") ||
			strings.HasPrefix(path, "/assets/") ||
			strings.HasPrefix(path, "/api/") ||
			strings.HasPrefix(path, "/report/") ||
			strings.HasPrefix(path, "/scheduler/")
	}
}

func isReportPath(path string) bool {
	path = strings.TrimSpace(path)
	return strings.HasPrefix(path, "/report/")
}

func isPublicPath(path string) bool {
	switch strings.TrimSpace(path) {
	case "/auth/login", "/auth/login/google", "/auth/callback", "/auth/logout", "/manifest.json", "/favicon.ico", "/icon.svg", "/icon-180.png", "/logo.jpeg", "/login.jpeg", "/login-bg.svg":
		return true
	case "/logo.svg":
		return true
	default:
		return false
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "unauthorized",
	})
}

func writeForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "forbidden",
	})
}
