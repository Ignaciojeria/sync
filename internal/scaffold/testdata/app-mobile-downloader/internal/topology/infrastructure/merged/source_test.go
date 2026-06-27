package merged

import (
	"context"
	"testing"
	"time"

	topologyapp "app-mobile-downloader/internal/topology/application"
)

type stub struct{ sessions []topologyapp.SyncSession }

func (s stub) ListSyncSessions(context.Context) ([]topologyapp.SyncSession, error) {
	return s.sessions, nil
}

func TestSourceListSyncSessionsOverlaysMetadata(t *testing.T) {
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo", ClientName: "BOOK-1", Source: "mutagen", Status: topologyapp.StatusRunning}}},
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ClientName: "user@example.com · BOOK-1", Source: "cli-file", LastSeenAt: now}}},
	)

	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].ClientName != "user@example.com · BOOK-1" {
		t.Fatalf("client = %q", sessions[0].ClientName)
	}
	if sessions[0].Source != "cli-file" {
		t.Fatalf("source = %q", sessions[0].Source)
	}
}
