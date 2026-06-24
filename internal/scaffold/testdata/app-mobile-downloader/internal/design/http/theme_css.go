package design

import (
	"fmt"
	"net/http"
	"strings"

	designapp "app-mobile-downloader/internal/design/application"
	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func registerThemeCSSHandler(s *server.Server, catalog designapp.Catalog) {
	fuego.Get(s.Server, "/design/theme/{id}", themeCSSHandler(catalog))
}

func themeCSSHandler(catalog designapp.Catalog) func(c fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id := strings.TrimSpace(c.PathParam("id"))
		id = catalog.ResolveThemeID(id)
		theme, ok := catalog.ThemeByID(id)
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusNotFound, Detail: "theme not found"}
		}

		css := designapp.CompileThemeCSS(theme)
		etag := fmt.Sprintf("\"%s\"", css.ETag)
		if strings.TrimSpace(c.Header("If-None-Match")) == etag {
			c.SetStatus(http.StatusNotModified)
			return nil, nil
		}

		c.SetHeader("Content-Type", "text/css; charset=utf-8")
		c.SetHeader("Cache-Control", "public, max-age=300")
		c.SetHeader("ETag", etag)
		_, err := c.Response().Write([]byte(css.Content))
		return nil, err
	}
}
