package postgresql

import (
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func newMockStore(t *testing.T) (*SessionStore, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	conn := &Connection{DB: sqlx.NewDb(db, "sqlmock")}
	return NewSessionStore(conn), mock
}

func TestNewSessionStore(t *testing.T) {
	conn := &Connection{DB: sqlx.NewDb(&sql.DB{}, "sqlmock")}
	store := NewSessionStore(conn)
	if store == nil {
		t.Fatal("expected session store to be created")
	}
	if store.db != conn {
		t.Fatal("expected session store to keep provided connection")
	}
}

func TestFindActiveSessionByID(t *testing.T) {
	query := regexp.QuoteMeta("SELECT s.id, s.user_id, s.access_token, s.refresh_token, s.id_token, s.expires_at, u.subject, u.email, u.display_name FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = $1 AND s.revoked_at IS NULL")

	t.Run("success", func(t *testing.T) {
		store, mock := newMockStore(t)
		expiresAt := time.Now().UTC()
		rows := sqlmock.NewRows([]string{"id", "user_id", "access_token", "refresh_token", "id_token", "expires_at", "subject", "email", "display_name"}).
			AddRow("sid-1", "uid-1", "access-token", "refresh-token", "id-token", expiresAt, "subject-1", "user@example.com", "User Name")
		mock.ExpectQuery(query).WithArgs("sid-1").WillReturnRows(rows)

		rec, err := store.FindActiveSessionByID("sid-1")
		if err != nil {
			t.Fatalf("FindActiveSessionByID() error = %v", err)
		}
		if rec.ID != "sid-1" || rec.UserID != "uid-1" || rec.Subject != "subject-1" {
			t.Fatalf("unexpected record: %+v", rec)
		}
		if !rec.Email.Valid || rec.Email.String != "user@example.com" {
			t.Fatalf("unexpected email: %+v", rec.Email)
		}
		if !rec.DisplayName.Valid || rec.DisplayName.String != "User Name" {
			t.Fatalf("unexpected display name: %+v", rec.DisplayName)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("query error", func(t *testing.T) {
		store, mock := newMockStore(t)
		mock.ExpectQuery(query).WithArgs("sid-2").WillReturnError(errors.New("query failed"))

		if _, err := store.FindActiveSessionByID("sid-2"); err == nil {
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
		store, mock := newMockStore(t)
		expiresAt := time.Now().UTC()
		mock.ExpectExec(stmt).
			WithArgs("access-token", "refresh-token", "id-token", &expiresAt, "sid-1").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := store.UpdateSessionTokens("sid-1", "access-token", "refresh-token", "id-token", &expiresAt)
		if err != nil {
			t.Fatalf("UpdateSessionTokens() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("exec error", func(t *testing.T) {
		store, mock := newMockStore(t)
		mock.ExpectExec(stmt).
			WithArgs("access-token", "refresh-token", "id-token", (*time.Time)(nil), "sid-2").
			WillReturnError(errors.New("exec failed"))

		if err := store.UpdateSessionTokens("sid-2", "access-token", "refresh-token", "id-token", nil); err == nil {
			t.Fatal("expected exec error")
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}
