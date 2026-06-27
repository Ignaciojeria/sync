package application

import (
	"context"
	"fmt"
	"strings"
)

type SyncSessionsStore interface {
	SyncSessionsSource
	UpsertSyncSession(context.Context, SyncSession) error
}

type UpsertSyncSessionInput struct {
	SessionID   string `json:"session_id"`
	ProjectName string `json:"project_name"`
	ClientName  string `json:"client_name"`
	Status      string `json:"status"`
	Source      string `json:"source"`
}

func (s *Service) UpsertSyncSession(ctx context.Context, input UpsertSyncSessionInput) error {
	if s.deps.SyncSessionsStore == nil {
		return fmt.Errorf("sync sessions store is not configured")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	projectName := strings.TrimSpace(input.ProjectName)
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if projectName == "" {
		return fmt.Errorf("project_name is required")
	}
	status := normalizeSessionStatus(input.Status)
	if status == "" {
		status = StatusRunning
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "cli"
	}
	return s.deps.SyncSessionsStore.UpsertSyncSession(ctx, SyncSession{
		SessionID:   sessionID,
		ProjectName: projectName,
		ClientName:  strings.TrimSpace(input.ClientName),
		Status:      status,
		Source:      source,
		LastSeenAt:  s.deps.Now(),
	})
}

func normalizeSessionStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", StatusRunning, StatusSyncing, StatusDegraded, StatusOffline:
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return StatusRunning
	}
}
