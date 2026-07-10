package scheduler

import (
	"scaffoldxd1/internal/shared"
	sharedpostgresql "scaffoldxd1/internal/shared/infrastructure/postgresql"
	"scaffoldxd1/internal/shared/server"
)

// Register wires scheduler HTTP routes and starts the cron runner.
func Register(s *server.Server, db *sharedpostgresql.Connection, hooks *shared.Hooks) error {
	jobConfigPageHandler(s, db)
	jobConfigAPIHandler(s, db)
	return startScheduler(db, hooks)
}
