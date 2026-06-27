package merged

import (
	"context"
	"testing"
	"time"

	topologyapp "app-mobile-downloader/internal/topology/application"
)

func TestSourceListSyncSessionsUsesOverlayWithoutMutagenBase(t *testing.T) {
	now := time.Date(2026, 6, 27, 3, 0, 0, 0, time.UTC)
	source := NewSource(
		stub{sessions: nil},
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo", ClientName: "user@example.com · BOOK", Source: "cli-file", LastSeenAt: now, Status: topologyapp.StatusRunning}}},
	)

	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].Source != "cli-file" {
		t.Fatalf("source = %q", sessions[0].Source)
	}
}
