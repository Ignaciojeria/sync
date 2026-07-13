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

func TestNewSourceDefaults(t *testing.T) {
	s := NewSource("", 0)
	if s.root != ".einar/sessions" {
		t.Errorf("root = %q, want .einar/sessions", s.root)
	}
	if s.ttl != 2*time.Minute {
		t.Errorf("ttl = %v", s.ttl)
	}
}

func TestSourceListSyncSessionsMissingDirReturnsEmpty(t *testing.T) {
	source := &Source{root: filepath.Join(t.TempDir(), "no-existe"), now: time.Now, ttl: time.Minute}
	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty, got %d", len(sessions))
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

func TestSourceListSyncSessionsSkipsNonJSONAndBadJSON(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	// .json con body roto → se ignora silenciosamente
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	// extensión no json → se ignora
	if err := os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	// session_id vacío → se ignora
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"session_id":"","project_name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewSource(dir, time.Minute)
	source.now = func() time.Time { return now }
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(got))
	}
}

func TestSourceListSyncSessionsFallsBackToFileModTime(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	// Sin last_seen_at: usa entry.Info().ModTime().
	content := `{"session_id":"sync-1","project_name":"demo","email":"u@e.com","hostname":"h"}`
	if err := os.WriteFile(filepath.Join(dir, "sync-1.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	source := NewSource(dir, time.Hour)
	source.now = func() time.Time { return now }
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
}

func TestClientNameVariants(t *testing.T) {
	if got := clientName("", "host"); got != "host" {
		t.Errorf("only host = %q", got)
	}
	if got := clientName("u@e.com", ""); got != "u@e.com" {
		t.Errorf("only email = %q", got)
	}
	if got := clientName("  ", "  "); got != "" {
		t.Errorf("both empty = %q", got)
	}
}

func TestFileSessionIDVariants(t *testing.T) {
	if got := fileSessionID("", "h"); got != "h" {
		t.Errorf("only host = %q", got)
	}
	if got := fileSessionID("s", ""); got != "s" {
		t.Errorf("only session = %q", got)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("  ", "", "x"); got != "x" {
		t.Errorf("first non-empty = %q", got)
	}
	if got := firstNonEmpty("  ", " y ", "x"); got != "y" {
		t.Errorf("first non-empty trimmed = %q", got)
	}
	if got := firstNonEmpty("  ", ""); got != "" {
		t.Errorf("all empty = %q", got)
	}
}
