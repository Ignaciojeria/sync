package auth

import (
	"net/http"

	authapp "app-mobile-downloader/internal/auth/application"
	authpostgresql "app-mobile-downloader/internal/auth/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(registerAuthLogout)

func registerAuthLogout(s *server.Server, conf configuration.Conf, store *authpostgresql.SessionRepository) {
	fuego.Get(s.Server, "/auth/logout", func(c fuego.ContextNoBody) (any, error) {
		if cookie, err := c.Request().Cookie("app_session_id"); err == nil {
			_ = store.RevokeSession(cookie.Value)
		}
		http.SetCookie(c.Response(), &http.Cookie{Name: "app_session_id", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: authapp.IsHTTPS(conf.OIDCRedirectURI), SameSite: http.SameSiteLaxMode})
		http.Redirect(c.Response(), c.Request(), "/", http.StatusFound)
		return nil, nil
	})
}
