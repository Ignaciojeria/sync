package auth

import "time"

type Session struct {
	ID           string
	UserID       string
	Subject      string
	Email        string
	DisplayName  string
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    *time.Time
}

type SessionRepository interface {
	FindActiveSessionByID(sessionID string) (Session, error)
	UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error
	CreateUserSession(identity Identity, resp CallbackResponse) (string, error)
	RevokeSession(sessionID string) error
}
