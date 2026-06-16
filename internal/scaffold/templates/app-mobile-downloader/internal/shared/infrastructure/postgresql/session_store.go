package postgresql

import (
	"database/sql"
	"time"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewSessionStore)

type SessionRecord struct {
	ID           string
	UserID       string
	Subject      string
	Email        sql.NullString
	DisplayName  sql.NullString
	AccessToken  sql.NullString
	RefreshToken sql.NullString
	IDToken      sql.NullString
	ExpiresAt    sql.NullTime
}

type SessionStore struct {
	db *Connection
}

func NewSessionStore(db *Connection) *SessionStore {
	return &SessionStore{db: db}
}

func (s *SessionStore) FindActiveSessionByID(sessionID string) (SessionRecord, error) {
	var rec SessionRecord
	query := "SELECT s.id, s.user_id, s.access_token, s.refresh_token, s.id_token, s.expires_at, u.subject, u.email, u.display_name " +
		"FROM sessions s JOIN users u ON u.id = s.user_id " +
		"WHERE s.id = $1 AND s.revoked_at IS NULL"
	row := s.db.QueryRowx(query, sessionID)
	err := row.Scan(&rec.ID, &rec.UserID, &rec.AccessToken, &rec.RefreshToken, &rec.IDToken, &rec.ExpiresAt, &rec.Subject, &rec.Email, &rec.DisplayName)
	return rec, err
}

func (s *SessionStore) UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error {
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
