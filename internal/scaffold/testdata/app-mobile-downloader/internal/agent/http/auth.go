package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	authapp "fixtests1/internal/auth/application"
	"fixtests1/internal/shared/server"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func refreshIDToken(ctx context.Context, lookup SessionLookup, oidc OIDCRefreshConfig, sessionID string) (authapp.Session, error) {
	rec, err := lookup.FindActiveSessionByID(sessionID)
	if err != nil {
		return authapp.Session{}, err
	}
	if strings.TrimSpace(rec.RefreshToken) == "" {
		return authapp.Session{}, fmt.Errorf("session %q sin refresh_token: el usuario debe volver a loguearse", sessionID)
	}
	if strings.TrimSpace(oidc.TokenEndpoint) == "" || strings.TrimSpace(oidc.ClientID) == "" {
		return authapp.Session{}, fmt.Errorf("OIDC_TOKEN_ENDPOINT / OIDC_CLIENT_ID no configurados en el web-server")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(rec.RefreshToken))
	form.Set("client_id", strings.TrimSpace(oidc.ClientID))
	if secret := strings.TrimSpace(oidc.ClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(oidc.TokenEndpoint), strings.NewReader(form.Encode()))
	if err != nil {
		return authapp.Session{}, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return authapp.Session{}, fmt.Errorf("refresh POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return authapp.Session{}, fmt.Errorf("refresh token exchange failed with status %d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return authapp.Session{}, fmt.Errorf("decode refresh response: %w", err)
	}
	accessToken := strings.TrimSpace(anyString(out["access_token"]))
	if accessToken == "" {
		return authapp.Session{}, fmt.Errorf("refresh returned empty access_token")
	}
	refreshToken := strings.TrimSpace(anyString(out["refresh_token"]))
	if refreshToken == "" {
		refreshToken = strings.TrimSpace(rec.RefreshToken)
	}
	idToken := strings.TrimSpace(anyString(out["id_token"]))
	if idToken == "" {
		idToken = strings.TrimSpace(rec.IDToken)
	}
	var expiresAt *time.Time
	if ei, ok := out["expires_in"].(float64); ok && int(ei) > 0 {
		ts := time.Now().Add(time.Duration(int(ei)) * time.Second)
		expiresAt = &ts
	}

	if err := lookup.UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken, expiresAt); err != nil {
		return authapp.Session{}, fmt.Errorf("update session tokens: %w", err)
	}
	rec.AccessToken = accessToken
	rec.RefreshToken = refreshToken
	rec.IDToken = idToken
	rec.ExpiresAt = expiresAt
	return rec, nil
}

func anyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprintf("%v", x)
	}
}

func authHandler(s *server.Server, lookup SessionLookup, oidc OIDCRefreshConfig) {
	server.Handle(s, "GET /agent/auth", auth(lookup, oidc))
}

func auth(lookup SessionLookup, oidc OIDCRefreshConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := sessionIDFromCookie(r)
		if !ok {
			writeError(w, server.HTTPError{Status: http.StatusUnauthorized, Detail: "missing app_session_id cookie"})
			return
		}
		rec, err := resolveFreshIDToken(r.Context(), lookup, oidc, sessionID)
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusUnauthorized, Detail: err.Error()})
			return
		}
		out := map[string]any{
			"token":     rec.IDToken,
			"email":     rec.Email,
			"subject":   rec.Subject,
			"expiresAt": rec.ExpiresAt,
		}
		if rec.ExpiresAt != nil {
			out["expiresAtUnix"] = rec.ExpiresAt.Unix()
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func sessionIDFromCookie(r *http.Request) (string, bool) {
	if r == nil {
		return "", false
	}
	cookie, err := r.Cookie("app_session_id")
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(cookie.Value)
	if v == "" {
		return "", false
	}
	return v, true
}

func resolveFreshIDToken(ctx context.Context, lookup SessionLookup, oidc OIDCRefreshConfig, sessionID string) (authapp.Session, error) {
	rec, err := lookup.FindActiveSessionByID(sessionID)
	if err != nil {
		log.Printf("/agent/auth: lookup session %s: %v", truncate(sessionID, 8), err)
		return authapp.Session{}, fmt.Errorf("lookup session: %w", err)
	}
	jwtExp := jwtExpClaim(rec.IDToken)
	jwtExpStr := "<vacío>"
	if !jwtExp.IsZero() {
		jwtExpStr = jwtExp.Format(time.RFC3339)
	}
	log.Printf("/agent/auth: session=%s subject=%q email=%q db.expiresAt=%v jwt.exp=%s", truncate(sessionID, 8), rec.Subject, rec.Email, rec.ExpiresAt, jwtExpStr)
	log.Printf("/agent/auth: idToken(jti+claims)=%s refreshToken=%s", jwtSummary(rec.IDToken), truncate(rec.RefreshToken, 24))
	needRefresh := false
	reason := ""
	if strings.TrimSpace(rec.IDToken) == "" {
		needRefresh = true
		reason = "id_token vacío"
	} else if jwtExp.IsZero() {
		needRefresh = true
		reason = "JWT sin claim exp parseable"
	} else if jwtExp.Before(time.Now()) {
		needRefresh = true
		reason = fmt.Sprintf("jwt.exp %s en el pasado", jwtExpStr)
	} else if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now()) {
		needRefresh = true
		reason = fmt.Sprintf("db.expiresAt %v en el pasado", rec.ExpiresAt)
	}
	if !needRefresh {
		return rec, nil
	}
	log.Printf("/agent/auth: refrescando (motivo: %s)", reason)
	fresh, err := refreshIDToken(ctx, lookup, oidc, sessionID)
	if err != nil {
		log.Printf("/agent/auth: refresh falló: %v", err)
		return authapp.Session{}, err
	}
	log.Printf("/agent/auth: refresh OK, nuevo jwt.exp=%s db.expiresAt=%v", jwtExpClaim(fresh.IDToken).Format(time.RFC3339), fresh.ExpiresAt)
	return fresh, nil
}

func jwtExpClaim(tokenString string) time.Time {
	parts := strings.Split(strings.TrimSpace(tokenString), ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return time.Time{}
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return time.Time{}
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(exp), 0).UTC()
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n] + "…"
}

func jwtSummary(tokenString string) string {
	parts := strings.Split(strings.TrimSpace(tokenString), ".")
	if len(parts) != 3 {
		if tokenString == "" {
			return "(empty)"
		}
		return "(no es un JWT: " + truncate(tokenString, 24) + ")"
	}
	dec := func(s string) map[string]any {
		switch len(s) % 4 {
		case 2:
			s += "=="
		case 3:
			s += "="
		}
		if raw, err := base64.RawURLEncoding.DecodeString(s); err == nil {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				return m
			}
		}
		if raw, err := base64.URLEncoding.DecodeString(s); err == nil {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				return m
			}
		}
		return nil
	}
	hdr := dec(parts[0])
	pl := dec(parts[1])
	parts2 := []string{}
	if hdr != nil {
		parts2 = append(parts2, fmt.Sprintf("alg=%v", hdr["alg"]), fmt.Sprintf("kid=%v", hdr["kid"]))
	}
	if pl != nil {
		parts2 = append(parts2,
			fmt.Sprintf("iss=%v", pl["iss"]),
			fmt.Sprintf("aud=%v", pl["aud"]),
			fmt.Sprintf("sub=%v", pl["sub"]),
			fmt.Sprintf("exp=%v", pl["exp"]),
			fmt.Sprintf("iat=%v", pl["iat"]),
		)
	}
	return strings.Join(parts2, " ")
}
