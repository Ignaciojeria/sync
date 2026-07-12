package auth

import (
	"net/http"
	"strings"

	authapp "testboi1/internal/auth/application"
	authpostgresql "testboi1/internal/auth/infrastructure/postgresql"
	"testboi1/internal/shared"
	"testboi1/internal/shared/configuration"
	mounted "testboi1/internal/shared/mounted"
	"testboi1/internal/shared/server"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/go-fuego/fuego"
)

func registerAuthCallback(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository, jwks keyfunc.Keyfunc) {
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

		resp, err := authapp.ExchangeAuthorizationCode(conf, code)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		identity, err := authapp.IdentityFromTokens(conf, jwks, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Detail: err.Error()}
		}
		if !shared.IsAllowedAnyEmail(identity.Email) {
			return nil, fuego.HTTPError{Status: http.StatusForbidden, Detail: "email sin acceso autorizado al sistema"}
		}
		sessionID, err := store.CreateUserSession(identity, resp)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
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
