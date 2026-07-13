package postgresql

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	authapp "fixtests1/internal/auth/application"
	sharedpostgresql "fixtests1/internal/shared/infrastructure/postgresql"
)

type SessionRepository struct {
	db *sharedpostgresql.Connection
}

func NewSessionRepository(db *sharedpostgresql.Connection) *SessionRepository {
	return &SessionRepository{db: db}
}

func (s *SessionRepository) FindActiveSessionByID(sessionID string) (authapp.Session, error) {
	var (
		rec          authapp.Session
		email        sql.NullString
		displayName  sql.NullString
		accessToken  sql.NullString
		refreshToken sql.NullString
		idToken      sql.NullString
		expiresAt    sql.NullTime
	)

	query := "SELECT s.id, s.user_id, s.access_token, s.refresh_token, s.id_token, s.expires_at, u.subject, u.email, u.display_name " +
		"FROM sessions s JOIN users u ON u.id = s.user_id " +
		"WHERE s.id = $1 AND s.revoked_at IS NULL"
	row := s.db.QueryRowx(query, sessionID)
	err := row.Scan(&rec.ID, &rec.UserID, &accessToken, &refreshToken, &idToken, &expiresAt, &rec.Subject, &email, &displayName)
	if err != nil {
		return authapp.Session{}, err
	}

	rec.Email = strings.TrimSpace(email.String)
	rec.DisplayName = strings.TrimSpace(displayName.String)
	rec.AccessToken = strings.TrimSpace(accessToken.String)
	rec.RefreshToken = strings.TrimSpace(refreshToken.String)
	rec.IDToken = strings.TrimSpace(idToken.String)
	if expiresAt.Valid {
		ts := expiresAt.Time
		rec.ExpiresAt = &ts
	}
	return rec, nil
}

func (s *SessionRepository) CreateUserSession(identity authapp.Identity, resp authapp.CallbackResponse) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("db connection is nil")
	}
	if strings.TrimSpace(identity.Subject) == "" {
		return "", fmt.Errorf("oidc subject is empty")
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var userID string
	userSQL := "INSERT INTO users (subject, email, display_name, created_at, updated_at) " +
		"VALUES ($1, $2, $3, NOW(), NOW()) " +
		"ON CONFLICT (subject) DO UPDATE SET email = EXCLUDED.email, display_name = EXCLUDED.display_name, updated_at = NOW() " +
		"RETURNING id"
	if err := tx.Get(&userID, userSQL, identity.Subject, nullableString(identity.Email), nullableString(identity.DisplayName)); err != nil {
		return "", err
	}

	var sessionID string
	var expiresAt any
	if resp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
	}
	sessionSQL := "INSERT INTO sessions (user_id, access_token, refresh_token, id_token, expires_at, created_at, updated_at) " +
		"VALUES ($1, $2, $3, $4, $5, NOW(), NOW()) RETURNING id"
	if err := tx.Get(&sessionID, sessionSQL, userID, nullableString(resp.AccessToken), nullableString(resp.RefreshToken), nullableString(resp.IDToken), expiresAt); err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return sessionID, nil
}

func (s *SessionRepository) RevokeSession(sessionID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	_, err := s.db.Exec("UPDATE sessions SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1", strings.TrimSpace(sessionID))
	return err
}

func (s *SessionRepository) UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error {
	_, err := s.db.Exec(
		"UPDATE sessions SET access_token=$1, refresh_token=$2, id_token=$3, expires_at=$4, updated_at=NOW() WHERE id=$5",
		accessToken,
		refreshToken,
		idToken,
		expiresAt,
		sessionID,
	)
	return err
}

func nullableString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
