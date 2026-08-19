package postgresql

import (
	"testing"

	"gitinittest5/internal/shared/configuration"
)

func TestNewConnectionReturnsErrorWhenDatabaseURLIsEmpty(t *testing.T) {
	_, err := NewConnection(configuration.Conf{DATABASE_URL: "  "})
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is empty")
	}
}

func TestParseDatabaseName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"postgres://user:pass@localhost:5432/appdb?sslmode=disable", "appdb", false},
		{"postgres://user:pass@localhost:5432/", "", false},
		{"://not-a-url", "", true},
	}
	for _, c := range cases {
		got, err := parseDatabaseName(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseDatabaseName(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseDatabaseName(%q) error = %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseDatabaseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRunMigrationsReturnsErrorWithNilDB(t *testing.T) {
	if err := (&Connection{name: "db"}).RunMigrations(); err == nil {
		t.Fatal("expected error when db connection is nil")
	}
	if err := (*Connection)(nil).RunMigrations(); err == nil {
		t.Fatal("expected nil connection to fail")
	}
}
