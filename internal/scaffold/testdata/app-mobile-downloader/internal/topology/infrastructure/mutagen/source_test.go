package mutagen

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	topologyapp "testboi1/internal/topology/application"
)

func TestParseSyncSessionsArray(t *testing.T) {
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	data := []byte(`[{"Identifier":"abc","Name":"workspace-gateway","Status":"Watching for changes"}]`)

	sessions, err := parseSyncSessions(data, now)
	if err != nil {
		t.Fatalf("parseSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].ProjectName != "workspace-gateway" {
		t.Fatalf("project = %q", sessions[0].ProjectName)
	}
	if sessions[0].Status != topologyapp.StatusRunning {
		t.Fatalf("status = %q", sessions[0].Status)
	}
}

func TestParseSyncSessionsObjectMap(t *testing.T) {
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	data := []byte(`{"sessions":{"abc":{"Identifier":"abc","Name":"workspace-gateway","Status":"Synchronizing"}}}`)

	sessions, err := parseSyncSessions(data, now)
	if err != nil {
		t.Fatalf("parseSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].Status != topologyapp.StatusSyncing {
		t.Fatalf("status = %q", sessions[0].Status)
	}
}

func TestSourceListSyncSessionsMutagenMissing(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	source := &Source{
		lookPath: func(string) (string, error) { return "", errors.New("missing") },
		run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run should not be called")
			return nil, nil
		},
	}

	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
}

func TestSourceListSyncSessionsUsesJSONOutput(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	source := &Source{
		lookPath: func(string) (string, error) { return "/usr/bin/mutagen", nil },
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(`[{"Identifier":"abc","Name":"workspace-gateway","Status":"Synchronizing"}]`), nil
		},
		now: func() time.Time { return now },
	}

	sessions, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSyncSessions() error = %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len(sessions) = %d", len(sessions))
	}
	if sessions[0].Source != "mutagen" {
		t.Fatalf("source = %q", sessions[0].Source)
	}
}

func TestResolveMutagenPathFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/mutagen"
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("MUTAGEN_PATH", path)

	resolved, err := resolveMutagenPath(func(string) (string, error) { return "", errors.New("missing") })
	if err != nil {
		t.Fatalf("resolveMutagenPath() error = %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved = %q", resolved)
	}
}
