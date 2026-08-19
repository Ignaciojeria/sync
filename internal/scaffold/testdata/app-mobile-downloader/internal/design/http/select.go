package design

import (
	designapp "gitinittest5/internal/design/application"
	"gitinittest5/internal/shared/server"
	"net/http"
	"strings"
)

func registerSelectThemeHandler(s *server.Server, catalog designapp.Catalog) {
	server.Post(s, "/design/select", selectThemeHandler(catalog))
}

func selectThemeHandler(catalog designapp.Catalog) func(c server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		if err := c.Request().ParseForm(); err != nil {
			return nil, server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
		}

		themeID := catalog.ResolveThemeID(strings.TrimSpace(c.Request().FormValue("theme_id")))
		http.SetCookie(c.Response(), &http.Cookie{
			Name:     designapp.ThemeCookieName,
			Value:    themeID,
			Path:     "/",
			HttpOnly: false,
			SameSite: http.SameSiteLaxMode,
		})
		c.SetHeader("HX-Refresh", "true")
		c.SetStatus(http.StatusNoContent)
		return nil, nil
	}
}
