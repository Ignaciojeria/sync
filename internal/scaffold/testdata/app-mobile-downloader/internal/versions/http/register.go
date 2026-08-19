package versions

import (
	authmiddleware "gitinittest5/internal/auth/middleware"
	versionsapp "gitinittest5/internal/versions/application"
	"gitinittest5/internal/shared/server"
)

// Register monta las rutas del módulo de versiones. Sigue el patrón
// de los demás bounded contexts: el servidor expone las páginas y
// esta función solo cablea el reader con la middleware de auth.
//
// Rutas:
//   GET /versions           → lista de merges recientes (últimas 30)
//   GET /versions/:sha      → detalle de una versión + diff vs HEAD
func Register(s *server.Server, reader versionsapp.Reader) {
	editor := authmiddleware.RequireEditor()
	server.Get(s, "/versions", listPage(reader), server.OptionMiddleware(editor))
	server.Get(s, "/versions/{sha}", detailPage(reader), server.OptionMiddleware(editor))
}