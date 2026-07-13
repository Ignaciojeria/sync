package design

import (
	designapp "fixtests1/internal/design/application"
	"fixtests1/internal/shared/server"
)

// Register wires design-system runtime routes onto the shared server.
func Register(s *server.Server, catalog designapp.Catalog) {
	registerThemeCSSHandler(s, catalog)
	registerSelectThemeHandler(s, catalog)
	registerPageHandler(s, catalog)
}
