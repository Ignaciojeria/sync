package design

import (
	"net/http"
	"strings"

	designapp "scaffoldxd1/internal/design/application"
	"scaffoldxd1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerSelectThemeHandler(s *server.Server, catalog designapp.Catalog) {
	fuego.Post(s.Server, "/design/select", selectThemeHandler(catalog))
}

func selectThemeHandler(catalog designapp.Catalog) func(c fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		if err := c.Request().ParseForm(); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
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
