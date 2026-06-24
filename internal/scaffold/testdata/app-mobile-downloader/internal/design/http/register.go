package design

import (
	designapp "app-mobile-downloader/internal/design/application"
	"app-mobile-downloader/internal/shared/server"
)

// Register wires design-system runtime routes onto the shared server.
func Register(s *server.Server, catalog designapp.Catalog) {
	registerThemeCSSHandler(s, catalog)
	registerSelectThemeHandler(s, catalog)
	registerPageHandler(s, catalog)
}
