package postgresql

import (
	"embed"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"app-mobile-downloader/internal/shared/configuration"

	"github.com/Ignaciojeria/ioc"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

var _ = ioc.Register(NewConnection)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Connection struct {
	*sqlx.DB
	name string
}

func NewConnection(conf configuration.Conf) (*Connection, error) {
	dsn := strings.TrimSpace(conf.DATABASE_URL)
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}

	name, err := parseDatabaseName(dsn)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	conn := &Connection{DB: db, name: name}
	if err := conn.RunMigrations(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	return conn, nil
}

func parseDatabaseName(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid DATABASE_URL format: %w", err)
	}
	return strings.TrimPrefix(u.Path, "/"), nil
}

func (c *Connection) RunMigrations() error {
	if c == nil || c.DB == nil {
		return fmt.Errorf("db connection is nil")
	}
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	driver, err := postgres.WithInstance(c.DB.DB, &postgres.Config{DatabaseName: c.name})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", d, c.name, driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}
	slog.Info("Database migrations validated/applied successfully")
	return nil
}

func runMigrations(db *sqlx.DB, dbName string) error {
	return (&Connection{DB: db, name: dbName}).RunMigrations()
}
