package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type SessionStatus string

type PreviewStatus string

const (
	SessionStatusIdle    SessionStatus = "idle"
	SessionStatusRunning SessionStatus = "running"
	SessionStatusError   SessionStatus = "error"
)

const (
	PreviewStatusNone     PreviewStatus = ""
	PreviewStatusStarting PreviewStatus = "starting"
	PreviewStatusLive     PreviewStatus = "live"
	PreviewStatusDown     PreviewStatus = "down"
)

var ErrSessionNotFound = errors.New("agent session not found")

// Session es la metadata persistible del chat del agente.
type Session struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	CWD             string        `json:"cwd"`
	WorkspacePath   string        `json:"workspacePath,omitempty"`
	Branch          string        `json:"branch,omitempty"`
	PreviewURL      string        `json:"previewURL,omitempty"`
	PreviewPort     int           `json:"previewPort,omitempty"`
	PreviewStatus   PreviewStatus `json:"previewStatus,omitempty"`
	PreviewHealth   string        `json:"previewHealth,omitempty"`
	Model           string        `json:"model"`
	PiSessionFile   string        `json:"piSessionFile"`
	Status          SessionStatus `json:"status"`
	LastPreview     string        `json:"lastPreview,omitempty"`
	LastPreviewKind string        `json:"lastPreviewKind,omitempty"`
	LastSeq         uint64        `json:"lastSeq,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type CreateSessionInput struct {
	Title string `json:"title"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type SessionStore interface {
	List(ctx context.Context) ([]Session, error)
	Create(ctx context.Context, session Session) error
	Get(ctx context.Context, id string) (Session, error)
	Update(ctx context.Context, session Session) error
	Delete(ctx context.Context, id string) error
}

func NewSession(id string, input CreateSessionInput, now time.Time) Session {
	id = strings.TrimSpace(id)
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = "Nueva sesión"
	}
	workspace := strings.TrimSpace(input.CWD)
	return Session{
		ID:            id,
		Title:         title,
		CWD:           workspace,
		WorkspacePath: workspace,
		Model:         strings.TrimSpace(input.Model),
		PiSessionFile: DefaultPiSessionFile(id),
		Status:        SessionStatusIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func DefaultPiSessionFile(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	path, err := filepath.Abs(filepath.Join("tmp", "agent-pi-sessions", id+".jsonl"))
	if err != nil {
		return filepath.Join("tmp", "agent-pi-sessions", id+".jsonl")
	}
	return path
}
