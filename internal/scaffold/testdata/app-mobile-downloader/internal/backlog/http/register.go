// Package http monta las rutas del backlog en el server compartido.
// La página principal se sirve bajo /backlog; las acciones HTMX
// (crear, mover, prioridad, eliminar, actualizar) viven bajo
// /backlog/cards/*.
package http

import (
	"gitinittest5/internal/backlog"
	"gitinittest5/internal/backlog/application"
	"gitinittest5/internal/backlog/infrastructure/fs"
	authmiddleware "gitinittest5/internal/auth/middleware"
	"gitinittest5/internal/shared/server"
)

// Register wires the backlog routes against an FS-backed Store.
//
// root es la ruta al bundle OKF (default: configurable vía env).
// Al arrancar, escribe index.md y AGENTS.md en el bundle si no
// existen (idempotente, no pisa ediciones manuales).
func Register(s *server.Server, root string) error {
	store, err := fs.NewStore(root)
	if err != nil {
		return err
	}
	if err := backlog.EnsureBundleMetadata(root); err != nil {
		// No fallamos el wiring: el bundle sigue funcional, solo
		// sin metadatos raíz.
		_ = err
	}
	svc := application.NewService(store)
	return registerHandlers(s, svc)
}

func registerHandlers(s *server.Server, svc *application.Service) error {
	// La creación de tarjetas es responsabilidad exclusiva del agente,
	// que las escribe como archivos .md directamente en el bundle.
	// Aquí solo gestionamos curaduría humana: mover, priorizar,
	// actualizar descripción y eliminar.
	requireEditor := server.OptionMiddleware(authmiddleware.RequireEditor())
	boardPageHandler(s, svc, requireEditor)
	cardDetailHandler(s, svc, requireEditor)
	moveCardHandler(s, svc, requireEditor)
	priorityCardHandler(s, svc, requireEditor)
	updateCardHandler(s, svc, requireEditor)
	deleteCardHandler(s, svc, requireEditor)
	return nil
}
