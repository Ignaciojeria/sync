package auth

import (
	"net/http"

	authapp "testboi1/internal/auth/application"
	authpostgresql "testboi1/internal/auth/infrastructure/postgresql"
	"testboi1/internal/shared/configuration"
	mounted "testboi1/internal/shared/mounted"
	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerAuthLogout(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository) {
	fuego.Get(s.Server, "/auth/logout", func(c fuego.ContextNoBody) (any, error) {
		if cookie, err := c.Request().Cookie("app_session_id"); err == nil {
			_ = store.RevokeSession(cookie.Value)
		}
		secure := authapp.IsHTTPS(conf.OIDCRedirectURI)
		http.SetCookie(c.Response(), &http.Cookie{Name: "app_session_id", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
		mounted.ClearReturnToCookie(c.Response(), secure)
		http.Redirect(c.Response(), c.Request(), mounted.App(mounted.Prefix(c.Request()), "/"), http.StatusFound)
		return nil, nil
	})
}
