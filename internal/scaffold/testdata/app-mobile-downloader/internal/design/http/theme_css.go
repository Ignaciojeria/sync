package design

import (
	designapp "gitinittest5/internal/design/application"
	"gitinittest5/internal/shared/server"
	"fmt"
	"net/http"
	"strings"
)

func registerThemeCSSHandler(s *server.Server, catalog designapp.Catalog) {
	server.Get(s, "/design/theme/{id}", themeCSSHandler(catalog))
}

func themeCSSHandler(catalog designapp.Catalog) func(c server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		id := strings.TrimSpace(c.PathParam("id"))
		id = catalog.ResolveThemeID(id)
		theme, ok := catalog.ThemeByID(id)
		if !ok {
			return nil, server.HTTPError{Status: http.StatusNotFound, Detail: "theme not found"}
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
