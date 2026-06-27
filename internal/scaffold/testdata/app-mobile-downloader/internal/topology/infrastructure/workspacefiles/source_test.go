package workspacefiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSourceListSyncSessions(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	content := `{"session_id":"sync-1","project_name":"demo","email":"user@example.com","hostname":"BOOK-1","last_seen_at":"2026-06-26T17:59:30Z"}`
	if err := os.WriteFile(filepath.Join(dir, "sync-1.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewSource(dir, time.Minute)
	source.now = func() time.Time { return now }

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
	if sessions[0].SessionID != "sync-1--BOOK-1" {
		t.Fatalf("session id = %q", sessions[0].SessionID)
	}
}

func TestSourceListSyncSessionsDropsStaleFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	content := `{"session_id":"sync-1","project_name":"demo","email":"user@example.com","hostname":"BOOK-1","last_seen_at":"2026-06-26T17:50:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "sync-1.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewSource(dir, time.Minute)
	source.now = func() time.Time { return now }

	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
}
