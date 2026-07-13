package postgresql

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	authapp "fixtests1/internal/auth/application"
	sharedpostgresql "fixtests1/internal/shared/infrastructure/postgresql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func newMockRepo(t *testing.T) (*SessionRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(db, "sqlmock")}
	return NewSessionRepository(conn), mock
}

func TestNewSessionRepository(t *testing.T) {
	conn := &sharedpostgresql.Connection{DB: sqlx.NewDb(&sql.DB{}, "sqlmock")}
	repo := NewSessionRepository(conn)
	if repo == nil {
		t.Fatal("expected session repository to be created")
	}
	if repo.db != conn {
		t.Fatal("expected session repository to keep provided connection")
	}
}

func TestFindActiveSessionByID(t *testing.T) {
	query := regexp.QuoteMeta("SELECT s.id, s.user_id, s.access_token, s.refresh_token, s.id_token, s.expires_at, u.subject, u.email, u.display_name FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = $1 AND s.revoked_at IS NULL")

	t.Run("success", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		expiresAt := time.Now().UTC()
		rows := sqlmock.NewRows([]string{"id", "user_id", "access_token", "refresh_token", "id_token", "expires_at", "subject", "email", "display_name"}).
			AddRow("sid-1", "uid-1", "access-token", "refresh-token", "id-token", expiresAt, "subject-1", "user@example.com", "User Name")
		mock.ExpectQuery(query).WithArgs("sid-1").WillReturnRows(rows)

		rec, err := repo.FindActiveSessionByID("sid-1")
		if err != nil {
			t.Fatalf("FindActiveSessionByID() error = %v", err)
		}
		if rec.ID != "sid-1" || rec.UserID != "uid-1" || rec.Subject != "subject-1" {
			t.Fatalf("unexpected record: %+v", rec)
		}
		if rec.Email != "user@example.com" {
			t.Fatalf("unexpected email: %q", rec.Email)
		}
		if rec.DisplayName != "User Name" {
			t.Fatalf("unexpected display name: %q", rec.DisplayName)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectQuery(query).WithArgs("sid-2").WillReturnError(errors.New("query failed"))

		if _, err := repo.FindActiveSessionByID("sid-2"); err == nil {
			t.Fatal("expected query error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func TestUpdateSessionTokens(t *testing.T) {
	stmt := regexp.QuoteMeta("UPDATE sessions SET access_token=$1, refresh_token=$2, id_token=$3, expires_at=$4, updated_at=NOW() WHERE id=$5")

	t.Run("success", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		expiresAt := time.Now().UTC()
		mock.ExpectExec(stmt).
			WithArgs("access-token", "refresh-token", "id-token", &expiresAt, "sid-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateSessionTokens("sid-1", "access-token", "refresh-token", "id-token", &expiresAt)
		if err != nil {
			t.Fatalf("UpdateSessionTokens() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectExec(stmt).
			WithArgs("access-token", "refresh-token", "id-token", (*time.Time)(nil), "sid-2").
			WillReturnError(errors.New("exec failed"))

		if err := repo.UpdateSessionTokens("sid-2", "access-token", "refresh-token", "id-token", nil); err == nil {
			t.Fatal("expected exec error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func TestCreateUserSession(t *testing.T) {
	t.Run("db nil", func(t *testing.T) {
		repo := &SessionRepository{db: nil}
		_, err := repo.CreateUserSession(authapp.Identity{Subject: "sub"}, authapp.CallbackResponse{})
		if err == nil || !strings.Contains(err.Error(), "db connection is nil") {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("empty subject", func(t *testing.T) {
		repo, _ := newMockRepo(t)
		_, err := repo.CreateUserSession(authapp.Identity{}, authapp.CallbackResponse{})
		if err == nil || !strings.Contains(err.Error(), "oidc subject is empty") {
			t.Fatalf("unexpected err: %v", err)
		}
	})

	t.Run("begin error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
		_, err := repo.CreateUserSession(authapp.Identity{Subject: "sub"}, authapp.CallbackResponse{})
		if err == nil {
			t.Fatal("expected begin error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("user insert error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnError(errors.New("user insert failed"))
		_, err := repo.CreateUserSession(authapp.Identity{Subject: "sub"}, authapp.CallbackResponse{})
		if err == nil {
			t.Fatal("expected user insert error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("session insert error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnError(errors.New("session insert failed"))
		_, err := repo.CreateUserSession(authapp.Identity{Subject: "sub"}, authapp.CallbackResponse{ExpiresIn: 10})
		if err == nil {
			t.Fatal("expected session insert error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("commit error", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
		mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
		_, err := repo.CreateUserSession(authapp.Identity{Subject: "sub"}, authapp.CallbackResponse{})
		if err == nil {
			t.Fatal("expected commit error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectBegin()
		mock.ExpectQuery("INSERT INTO users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
		mock.ExpectQuery("INSERT INTO sessions").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
		mock.ExpectCommit()
		sessionID, err := repo.CreateUserSession(authapp.Identity{Subject: "sub", Email: "user@example.com", DisplayName: "User"}, authapp.CallbackResponse{AccessToken: "a", RefreshToken: "r", IDToken: "i", ExpiresIn: 30})
		if err != nil {
			t.Fatalf("CreateUserSession() error = %v", err)
		}
		if sessionID != "s1" {
			t.Fatalf("sessionID = %q, want s1", sessionID)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})
}

func TestRevokeSession(t *testing.T) {
	stmt := regexp.QuoteMeta("UPDATE sessions SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1")

	t.Run("success", func(t *testing.T) {
		repo, mock := newMockRepo(t)
		mock.ExpectExec(stmt).WithArgs("sid-1").WillReturnResult(sqlmock.NewResult(0, 1))
		if err := repo.RevokeSession("sid-1"); err != nil {
			t.Fatalf("RevokeSession() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("nil db returns nil", func(t *testing.T) {
		repo := &SessionRepository{db: nil}
		if err := repo.RevokeSession("sid-1"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty session id returns nil", func(t *testing.T) {
		repo, _ := newMockRepo(t)
		if err := repo.RevokeSession("  "); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})
}
