package scheduler

import (
	"app-mobile-downloader/internal/shared"
	sharedpostgresql "app-mobile-downloader/internal/shared/infrastructure/postgresql"
	"app-mobile-downloader/internal/shared/server"
)

// Register wires scheduler HTTP routes and starts the cron runner.
func Register(s *server.Server, db *sharedpostgresql.Connection, hooks *shared.Hooks) error {
	jobConfigPageHandler(s, db)
	jobConfigAPIHandler(s, db)
	return startScheduler(db, hooks)
}
