package memory

import (
	"context"
	"testing"
	"time"

	topologyapp "app-mobile-downloader/internal/topology/application"
)

func TestSyncSessionsStoreUpsertAndList(t *testing.T) {
	now := time.Date(2026, 6, 26, 19, 0, 0, 0, time.UTC)
	store := NewSyncSessionsStore(time.Minute)
	store.now = func() time.Time { return now }

	if err := store.UpsertSyncSession(context.Background(), topologyapp.SyncSession{
		SessionID:   "s1",
		ProjectName: "workspace-gateway",
		Status:      topologyapp.StatusRunning,
		Source:      "cli",
	}); err != nil {
		t.Fatalf("UpsertSyncSession() error = %v", err)
	}

	sessions, err := store.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].StartedAt.IsZero() {
		t.Fatal("expected started_at")
	}
}

func TestSyncSessionsStoreDropsStaleSessions(t *testing.T) {
	now := time.Date(2026, 6, 26, 19, 0, 0, 0, time.UTC)
	store := NewSyncSessionsStore(time.Minute)
	store.now = func() time.Time { return now }
	_ = store.UpsertSyncSession(context.Background(), topologyapp.SyncSession{
		SessionID:   "s1",
		ProjectName: "workspace-gateway",
		Status:      topologyapp.StatusRunning,
		LastSeenAt:  now.Add(-2 * time.Minute),
	})

	sessions, err := store.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
}

func TestSyncSessionsStoreDropsOfflineSessions(t *testing.T) {
	store := NewSyncSessionsStore(time.Minute)
	_ = store.UpsertSyncSession(context.Background(), topologyapp.SyncSession{
		SessionID:   "s1",
		ProjectName: "workspace-gateway",
		Status:      topologyapp.StatusOffline,
	})

	sessions, err := store.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
}
