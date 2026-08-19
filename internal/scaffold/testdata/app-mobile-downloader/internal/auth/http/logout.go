package auth

import (
	authapp "gitinittest5/internal/auth/application"
	authpostgresql "gitinittest5/internal/auth/infrastructure/postgresql"
	"gitinittest5/internal/shared/configuration"
	mounted "gitinittest5/internal/shared/mounted"
	"gitinittest5/internal/shared/server"
	"net/http"
)

func registerAuthLogout(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository) {
	server.Get(s, "/auth/logout", func(c server.ContextNoBody) (any, error) {
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
