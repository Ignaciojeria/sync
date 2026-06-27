package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type serviceSourceStub struct {
	services []ServiceNode
	err      error
}

func (s serviceSourceStub) ListServices(context.Context) ([]ServiceNode, error) {
	return s.services, s.err
}

type syncSourceStub struct {
	sessions []SyncSession
	err      error
}

func (s syncSourceStub) ListSyncSessions(context.Context) ([]SyncSession, error) {
	return s.sessions, s.err
}

func TestServiceGetSnapshotDefaults(t *testing.T) {
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	service := NewServiceWithDeps(ServiceDeps{Now: func() time.Time { return now }})

	snapshot, err := service.GetSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if snapshot.Workspace.Name != "workspace-gateway" {
		t.Fatalf("workspace name = %q", snapshot.Workspace.Name)
	}
	if snapshot.Workspace.Status != StatusRunning {
		t.Fatalf("workspace status = %q", snapshot.Workspace.Status)
	}
	if !snapshot.GeneratedAt.Equal(now) {
		t.Fatalf("generated at = %v", snapshot.GeneratedAt)
	}
}

func TestServiceGetSnapshotDegradedFromService(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{
		WorkspaceName:  "sync-run",
		ServicesSource: serviceSourceStub{services: []ServiceNode{{Name: "PostgreSQL", Status: StatusDegraded}}},
	})

	snapshot, err := service.GetSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if snapshot.Workspace.Status != StatusDegraded {
		t.Fatalf("workspace status = %q", snapshot.Workspace.Status)
	}
}

func TestServiceGetSnapshotSyncingFromSession(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{
		SyncSessionsSource: syncSourceStub{sessions: []SyncSession{{ProjectName: "workspace-gateway", Status: StatusSyncing}}},
	})

	snapshot, err := service.GetSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if snapshot.Workspace.Status != StatusSyncing {
		t.Fatalf("workspace status = %q", snapshot.Workspace.Status)
	}
}

func TestServiceGetSnapshotServiceError(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{ServicesSource: serviceSourceStub{err: errors.New("boom")}})
	if _, err := service.GetSnapshot(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceGetSnapshotSyncError(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{SyncSessionsSource: syncSourceStub{err: errors.New("boom")}})
	if _, err := service.GetSnapshot(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
