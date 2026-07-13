package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

type syncStoreStub struct {
	upsert func(context.Context, SyncSession) error
}

func (s syncStoreStub) ListSyncSessions(context.Context) ([]SyncSession, error) {
	return nil, nil
}

func (s syncStoreStub) UpsertSyncSession(ctx context.Context, sess SyncSession) error {
	if s.upsert != nil {
		return s.upsert(ctx, sess)
	}
	return nil
}

func TestUpsertSyncSessionRequiresStore(t *testing.T) {
	service := &Service{}
	if err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{SessionID: "s1", ProjectName: "p"}); err == nil {
		t.Fatal("expected error when store is nil")
	}
}

func TestUpsertSyncSessionRequiresSessionID(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{SyncSessionsStore: syncStoreStub{}})
	err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{ProjectName: "p"})
	if err == nil || err.Error() != "session_id is required" {
		t.Fatalf("expected session_id required error, got %v", err)
	}
}

func TestUpsertSyncSessionRequiresProjectName(t *testing.T) {
	service := NewServiceWithDeps(ServiceDeps{SyncSessionsStore: syncStoreStub{}})
	err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{SessionID: "s1"})
	if err == nil || err.Error() != "project_name is required" {
		t.Fatalf("expected project_name required error, got %v", err)
	}
}

func TestUpsertSyncSessionDefaultsStatus(t *testing.T) {
	var captured SyncSession
	store := syncStoreStub{upsert: func(_ context.Context, sess SyncSession) error {
		captured = sess
		return nil
	}}
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	service := NewServiceWithDeps(ServiceDeps{
		SyncSessionsStore: store,
		Now:               func() time.Time { return now },
	})

	if err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{
		SessionID:   "s1",
		ProjectName: "p",
		ClientName:  "cli-x",
	}); err != nil {
		t.Fatalf("UpsertSyncSession() error = %v", err)
	}
	if captured.Status != StatusRunning {
		t.Errorf("default status = %q, want %q", captured.Status, StatusRunning)
	}
	if captured.Source != "cli" {
		t.Errorf("default source = %q, want cli", captured.Source)
	}
	if captured.ClientName != "cli-x" {
		t.Errorf("client name not propagated: %q", captured.ClientName)
	}
	if !captured.LastSeenAt.Equal(now) {
		t.Errorf("LastSeenAt = %v, want %v", captured.LastSeenAt, now)
	}
}

func TestUpsertSyncSessionPropagatesStoreError(t *testing.T) {
	store := syncStoreStub{upsert: func(context.Context, SyncSession) error {
		return errors.New("disk full")
	}}
	service := NewServiceWithDeps(ServiceDeps{SyncSessionsStore: store})
	err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{
		SessionID:   "s1",
		ProjectName: "p",
	})
	if err == nil || err.Error() != "disk full" {
		t.Fatalf("expected disk full, got %v", err)
	}
}

func TestNormalizeSessionStatus(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""}, // empty → vacío; UpsertSyncSession lo convierte a running.
		{"running", StatusRunning},
		{"SYNCING", StatusSyncing},
		{" degraded ", StatusDegraded},
		{"offline", StatusOffline},
		{"unknown", StatusRunning},
	}
	for _, c := range cases {
		if got := normalizeSessionStatus(c.in); got != c.want {
			t.Errorf("normalizeSessionStatus(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUpsertSyncSessionAcceptsAllKnownStatuses(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"running", StatusRunning},
		{"syncing", StatusSyncing},
		{"degraded", StatusDegraded},
		{"offline", StatusOffline},
	}
	for _, c := range cases {
		var captured SyncSession
		store := syncStoreStub{upsert: func(_ context.Context, sess SyncSession) error {
			captured = sess
			return nil
		}}
		service := NewServiceWithDeps(ServiceDeps{SyncSessionsStore: store})
		if err := service.UpsertSyncSession(context.Background(), UpsertSyncSessionInput{
			SessionID:   "s1",
			ProjectName: "p",
			Status:      c.in,
		}); err != nil {
			t.Fatalf("UpsertSyncSession(%q) error = %v", c.in, err)
		}
		if captured.Status != c.want {
			t.Errorf("status for %q = %q, want %q", c.in, captured.Status, c.want)
		}
	}
}
