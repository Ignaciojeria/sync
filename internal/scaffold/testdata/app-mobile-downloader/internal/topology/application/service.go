package application

import (
	"context"
	"strings"
	"time"
)

type ServicesSource interface {
	ListServices(context.Context) ([]ServiceNode, error)
}

type SyncSessionsSource interface {
	ListSyncSessions(context.Context) ([]SyncSession, error)
}

type SnapshotReader interface {
	GetSnapshot(context.Context) (Snapshot, error)
}

type ServiceDeps struct {
	WorkspaceName      string
	WorkspaceSummary   string
	ServicesSource     ServicesSource
	SyncSessionsSource SyncSessionsSource
	SyncSessionsStore  SyncSessionsStore
	Now                func() time.Time
}

type Service struct {
	deps ServiceDeps
}

func NewServiceWithDeps(deps ServiceDeps) *Service {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if strings.TrimSpace(deps.WorkspaceName) == "" {
		deps.WorkspaceName = "workspace-gateway"
	}
	if strings.TrimSpace(deps.WorkspaceSummary) == "" {
		deps.WorkspaceSummary = "Runtime persistente del workspace"
	}
	return &Service{deps: deps}
}

func (s *Service) GetSnapshot(ctx context.Context) (Snapshot, error) {
	now := s.deps.Now()
	snapshot := Snapshot{
		Workspace: Workspace{
			Name:      s.deps.WorkspaceName,
			Status:    StatusRunning,
			Summary:   s.deps.WorkspaceSummary,
			UpdatedAt: now,
		},
		GeneratedAt: now,
	}

	if s.deps.ServicesSource != nil {
		services, err := s.deps.ServicesSource.ListServices(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.Services = services
		if hasStatus(services, StatusDegraded, StatusOffline) {
			snapshot.Workspace.Status = StatusDegraded
		}
	}

	if s.deps.SyncSessionsSource != nil {
		sessions, err := s.deps.SyncSessionsSource.ListSyncSessions(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		snapshot.SyncSessions = sessions
		if hasSyncingSession(sessions) && snapshot.Workspace.Status == StatusRunning {
			snapshot.Workspace.Status = StatusSyncing
		}
	}

	return snapshot, nil
}

func hasStatus(services []ServiceNode, statuses ...string) bool {
	for _, service := range services {
		for _, status := range statuses {
			if service.Status == status {
				return true
			}
		}
	}
	return false
}

func hasSyncingSession(sessions []SyncSession) bool {
	for _, session := range sessions {
		if session.Status == StatusSyncing {
			return true
		}
	}
	return false
}
