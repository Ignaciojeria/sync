package merged

import (
	"context"
	"testing"
	"time"

	topologyapp "gitinittest5/internal/topology/application"
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

func TestSourceListSyncSessionsNilReceiver(t *testing.T) {
	var s *Source
	got, err := s.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Errorf("got = %v", got)
	}
}

func TestSourceListSyncSessionsNilPrimary(t *testing.T) {
	source := &Source{}
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Errorf("got = %v", got)
	}
}

func TestSourceListSyncSessionsPrimaryError(t *testing.T) {
	errStub := errStubFn{err: context.DeadlineExceeded}
	source := NewSource(errStub)
	if _, err := source.ListSyncSessions(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestSourceListSyncSessionsOverlayAddsNewSession(t *testing.T) {
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo"}}},
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-2", ProjectName: "demo"}}},
	)
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d", len(got))
	}
}

func TestSourceListSyncSessionsOverlayIgnoresEmptyID(t *testing.T) {
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo"}}},
		stub{sessions: []topologyapp.SyncSession{{SessionID: "  ", ProjectName: "demo"}}},
	)
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d", len(got))
	}
}

func TestSourceListSyncSessionsPrimaryIgnoresEmptyIDs(t *testing.T) {
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "", ProjectName: "demo"}, {SessionID: "sync-2", ProjectName: "demo"}}},
	)
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len = %d", len(got))
	}
}

func TestSourceListSyncSessionsOverlayError(t *testing.T) {
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo"}}},
		errStubFn{err: context.DeadlineExceeded},
	)
	if _, err := source.ListSyncSessions(context.Background()); err == nil {
		t.Fatal("expected error from overlay")
	}
}

func TestSourceListSyncSessionsSkipsNilOverlay(t *testing.T) {
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo"}}},
		nil,
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-2", ProjectName: "demo"}}},
	)
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d", len(got))
	}
}

func TestSourceListSyncSessionsOverlayReplacesStartedAt(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	source := NewSource(
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo", StartedAt: t0}}},
		stub{sessions: []topologyapp.SyncSession{{SessionID: "sync-1", ProjectName: "demo", StartedAt: t1, LastSeenAt: t1}}},
	)
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !got[0].StartedAt.Equal(t1) {
		t.Errorf("StartedAt = %v, want %v", got[0].StartedAt, t1)
	}
}

type errStubFn struct{ err error }

func (e errStubFn) ListSyncSessions(context.Context) ([]topologyapp.SyncSession, error) {
	return nil, e.err
}
