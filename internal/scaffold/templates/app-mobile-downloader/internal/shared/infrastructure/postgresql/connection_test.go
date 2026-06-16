package postgresql

import (
	"testing"

	"app-mobile-downloader/internal/shared/configuration"
)

func TestNewConnectionReturnsErrorWhenDatabaseURLIsEmpty(t *testing.T) {
	_, err := NewConnection(configuration.Conf{DATABASE_URL: "  "})
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestParseDatabaseName(t *testing.T) {
	got, err := parseDatabaseName("postgres://user:pass@localhost:5432/appdb?sslmode=disable")
	if err != nil {
		t.Fatalf("parseDatabaseName() error = %v", err)
	}
	if got != "appdb" {
		t.Fatalf("parseDatabaseName() = %q", got)
	}
}

func TestRunMigrationsReturnsErrorWithNilDB(t *testing.T) {
	if err := runMigrations(nil, "db"); err == nil {
		t.Fatal("expected error when db connection is nil")
	}
	if err := (*Connection)(nil).RunMigrations(); err == nil {
		t.Fatal("expected nil connection to fail")
	}
}
