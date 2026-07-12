package postgresql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	sharedpostgresql "testboi1/internal/shared/infrastructure/postgresql"
	"testboi1/pkg/agent/application"
)

type RuntimeEventRow struct {
	Offset    uint64          `db:"offset"`
	SessionID string          `db:"session_id"`
	Kind      string          `db:"kind"`
	Payload   json.RawMessage `db:"payload"`
	CreatedAt time.Time       `db:"created_at"`
}

type RuntimeEventsStore struct {
	db *sharedpostgresql.Connection
}

func NewRuntimeEventsStore(db *sharedpostgresql.Connection) *RuntimeEventsStore {
	return &RuntimeEventsStore{db: db}
}

func (s *RuntimeEventsStore) Append(ctx context.Context, sessionID string, kind string, payload any) (uint64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("runtime events store: db is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	kind = strings.TrimSpace(kind)
	if sessionID == "" {
		return 0, fmt.Errorf("runtime events store: session_id is empty")
	}
	if kind == "" {
		kind = "pi"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("runtime events store: marshal payload: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// ponytail: lock por sesión para asignar offset monotónico sin carreras.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, sessionID); err != nil {
		return 0, err
	}

	var offset uint64
	query := `
		INSERT INTO agent_runtime_events (session_id, event_offset, kind, payload)
		VALUES (
			$1,
			COALESCE((SELECT max(event_offset) + 1 FROM agent_runtime_events WHERE session_id = $1), 1),
			$2,
			$3::jsonb
		)
		RETURNING event_offset
	`
	if err := tx.QueryRowContext(ctx, query, sessionID, kind, body).Scan(&offset); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return offset, nil
}

func (s *RuntimeEventsStore) ListAfter(ctx context.Context, sessionID string, after uint64, limit int) ([]application.RuntimeEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("runtime events store: db is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	const query = `
		SELECT event_offset as offset, kind, payload
		FROM agent_runtime_events
		WHERE session_id = $1 AND event_offset > $2
		ORDER BY event_offset ASC
		LIMIT $3
	`
	var rows []application.RuntimeEventRecord
	if err := s.db.SelectContext(ctx, &rows, query, sessionID, after, limit); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *RuntimeEventsStore) ListSession(ctx context.Context, sessionID string) ([]application.RuntimeEventRecord, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("runtime events store: db is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	const query = `
		SELECT event_offset as offset, kind, payload
		FROM agent_runtime_events
		WHERE session_id = $1
		ORDER BY event_offset ASC
	`
	var rows []application.RuntimeEventRecord
	if err := s.db.SelectContext(ctx, &rows, query, sessionID); err != nil {
		return nil, err
	}
	return rows, nil
}
