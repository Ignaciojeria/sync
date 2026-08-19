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
	// OwnerEmail identifica al humano que está chateando. Lo
	// popula el HTTP handler desde el JWT en Create; el
	// Manager lo usa para construir la MemoryKey del provider
	// de memoria al hacer flush de un turno.
	OwnerEmail      string        `json:"ownerEmail,omitempty"`
	// AgentID identifica qué agente corre esta sesión (ej.
	// "develop"). Se persiste desde CreateSessionInput.AgentID
	// vía ResolveAgentID. Vacío se interpreta como default.
	// Lo consume pirpc para sembrar el .pi y AGENTS.md del
	// workspace agents/<id>/ correspondiente.
	AgentID         string        `json:"agentId,omitempty"`
	CWD             string        `json:"cwd"`
	WorkspacePath   string        `json:"workspacePath,omitempty"`
	SourcePath      string        `json:"sourcePath,omitempty"`
	Branch          string        `json:"branch,omitempty"`
	BaseBranch      string        `json:"baseBranch,omitempty"`
	BaseCommit      string        `json:"baseCommit,omitempty"`
	MergedAt        *time.Time    `json:"mergedAt,omitempty"`
	MergedCommit    string        `json:"mergedCommit,omitempty"`
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
	// OwnerEmail identifica al humano que crea la session.
	// Se persiste en Session.OwnerEmail y se usa para construir
	// la MemoryKey del provider de memoria al flushear un turno.
	OwnerEmail string `json:"ownerEmail,omitempty"`
	// AgentID identifica qué agente se va a correr (ej.
	// "develop"). Si vacío, el Manager lo resuelve al default
	// del registry (DefaultAgentID). El runner siembra el
	// .pi/ y AGENTS.md de agents/<AgentID>/ en el sandbox.
	AgentID string `json:"agentId,omitempty"`
}

type MergeResult struct {
	BaseBranch    string `json:"baseBranch"`
	PreviewBranch string `json:"previewBranch"`
	Commit        string `json:"commit"`
	// NoChanges se setea cuando el merge no tenía nada nuevo que
	// integrar (preview-branch == base-branch, o todos los commits
	// de preview ya están en base). El handler debe mostrar el
	// estado "Up to date" en el bar sin crear una sesión nueva.
	// Si Commit es vacío y NoChanges es true, no hubo integración
	// real y la sesión NO se marca como mergeada.
	NoChanges bool `json:"noChanges,omitempty"`
}

type ApplyResult struct {
	SourcePath  string `json:"sourcePath"`
	PreviewPath string `json:"previewPath"`
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
	agentID, _ := ResolveAgentID(input.AgentID)
	return Session{
		ID:            id,
		Title:         title,
		OwnerEmail:    strings.TrimSpace(input.OwnerEmail),
		AgentID:       agentID,
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
