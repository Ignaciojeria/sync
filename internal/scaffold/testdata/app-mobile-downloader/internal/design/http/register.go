package design

import (
	designapp "testboi1/internal/design/application"
	"testboi1/internal/shared/server"
)

// Register wires design-system runtime routes onto the shared server.
func Register(s *server.Server, catalog designapp.Catalog) {
	registerThemeCSSHandler(s, catalog)
	registerSelectThemeHandler(s, catalog)
	registerPageHandler(s, catalog)
}
