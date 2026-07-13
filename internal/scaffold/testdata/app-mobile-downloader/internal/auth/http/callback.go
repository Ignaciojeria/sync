package auth

import (
	authapp "fixtests1/internal/auth/application"
	authpostgresql "fixtests1/internal/auth/infrastructure/postgresql"
	"fixtests1/internal/shared"
	"fixtests1/internal/shared/configuration"
	mounted "fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
	"github.com/MicahParks/keyfunc/v3"
	"net/http"
	"strings"
)

func registerAuthCallback(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository, jwks keyfunc.Keyfunc) {
	server.Get(s, "/auth/callback", func(c server.ContextNoBody) (any, error) {
		state := strings.TrimSpace(c.QueryParam("state"))
		code := strings.TrimSpace(c.QueryParam("code"))
		if code == "" {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: "missing code"}
		}

		stateCookie, err := c.Request().Cookie("oidc_state")
		if err != nil || strings.TrimSpace(stateCookie.Value) == "" || stateCookie.Value != state {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: "invalid oauth state"}
		}

		resp, err := authapp.ExchangeAuthorizationCode(conf, code)
		if err != nil {
			return nil, server.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		identity, err := authapp.IdentityFromTokens(conf, jwks, resp)
		if err != nil {
			return nil, server.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		if !shared.IsAllowedAnyEmail(identity.Email) {
			return nil, server.HTTPError{Status: http.StatusForbidden, Detail: "email sin acceso autorizado al sistema"}
		}
		sessionID, err := store.CreateUserSession(identity, resp)
		if err != nil {
			return nil, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		secure := authapp.IsHTTPS(conf.OIDCRedirectURI)
		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "oidc_state",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "app_session_id",
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		redirectTo := mounted.ReadReturnTo(c.Request())
		mounted.ClearReturnToCookie(c.Response(), secure)
		if redirectTo == "" {
			redirectTo = authapp.PostLoginRedirectPath
		}
		http.Redirect(c.Response(), c.Request(), redirectTo, http.StatusFound)
		return nil, nil
	})
}
