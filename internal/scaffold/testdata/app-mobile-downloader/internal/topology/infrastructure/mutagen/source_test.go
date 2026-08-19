package mutagen

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	topologyapp "gitinittest5/internal/topology/application"
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

func TestResolveMutagenPathFromLookPath(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	resolved, err := resolveMutagenPath(func(name string) (string, error) {
		if name == "mutagen" {
			return "/usr/bin/mutagen", nil
		}
		return "", errors.New("no")
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resolved != "/usr/bin/mutagen" {
		t.Errorf("resolved = %q", resolved)
	}
}

func TestResolveMutagenPathFallsBackToLocal(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	name := binaryName("mutagen")
	if err := os.WriteFile(name, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveMutagenPath(func(string) (string, error) { return "", errors.New("missing") })
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if resolved == "" {
		t.Errorf("resolved should not be empty")
	}
}

func TestResolveMutagenPathMissing(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	if _, err := resolveMutagenPath(func(string) (string, error) { return "", errors.New("missing") }); err == nil {
		t.Fatal("expected error when mutagen not found")
	}
}

func TestDeriveStatusBranches(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want string
	}{
		{"watching", map[string]any{"Status": "Watching for changes"}, topologyapp.StatusRunning},
		{"syncing", map[string]any{"Status": "Synchronizing files"}, topologyapp.StatusSyncing},
		{"scanning", map[string]any{"Status": "Scanning"}, topologyapp.StatusSyncing},
		{"staging", map[string]any{"Status": "Staging files"}, topologyapp.StatusRunning}, // ponytail: "staging" no contiene el substring "stage"; cae al default
		{"stage-files", map[string]any{"Status": "Stage files"}, topologyapp.StatusSyncing},
		{"paused", map[string]any{"Status": "Paused"}, topologyapp.StatusOffline},
		{"halted", map[string]any{"Status": "Halted"}, topologyapp.StatusOffline},
		{"offline-text", map[string]any{"Status": "Offline"}, topologyapp.StatusOffline},
		{"degraded", map[string]any{"Status": "Degraded"}, topologyapp.StatusDegraded},
		{"stalled", map[string]any{"Status": "Stalled"}, topologyapp.StatusDegraded},
		{"problem", map[string]any{"Status": "Problem detected"}, topologyapp.StatusDegraded},
		{"error", map[string]any{"Status": "Internal error"}, topologyapp.StatusDegraded},
		{"empty", map[string]any{}, ""},
		{"paused-flag", map[string]any{"Paused": true}, topologyapp.StatusOffline},
		{"paused-flag-lower", map[string]any{"paused": true}, topologyapp.StatusOffline},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveStatus(c.in); got != c.want {
				t.Errorf("deriveStatus(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestBinaryName(t *testing.T) {
	if got := binaryName("mutagen"); got == "" {
		t.Error("empty result")
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(binaryName("mutagen"), ".exe") {
			t.Error("windows binary should have .exe")
		}
	}
}

func TestFirstStringVariants(t *testing.T) {
	m := map[string]any{"A": "x", "b": "y", "c": 42, "d": "  z  "}
	if got := firstString(m, "A"); got != "x" {
		t.Errorf("A = %q", got)
	}
	if got := firstString(m, "missing"); got != "" {
		t.Errorf("missing = %q", got)
	}
	if got := firstString(m, "c"); got != "" {
		t.Errorf("non-string value = %q", got)
	}
	if got := firstString(m, "d"); got != "z" {
		t.Errorf("trim = %q", got)
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

func TestSourceListSyncSessionsEmptyJSON(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	source := &Source{
		lookPath: func(string) (string, error) { return "/usr/bin/mutagen", nil },
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte(""), nil
		},
	}
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Errorf("got = %v", got)
	}
}

func TestSourceListSyncSessionsInvalidJSON(t *testing.T) {
	t.Setenv("MUTAGEN_PATH", "")
	source := &Source{
		lookPath: func(string) (string, error) { return "/usr/bin/mutagen", nil },
		run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("not json"), nil
		},
	}
	got, err := source.ListSyncSessions(context.Background())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != nil {
		t.Errorf("got = %v", got)
	}
}

func TestParseSyncSessionsEmpty(t *testing.T) {
	if _, err := parseSyncSessions([]byte(`[]`), time.Now()); err == nil {
		t.Error("expected error for empty sessions")
	}
}

func TestWalkSessionsDeeplyNested(t *testing.T) {
	now := time.Now()
	seen := map[string]topologyapp.SyncSession{}
	data := map[string]any{
		"level1": map[string]any{
			"level2": []any{
				map[string]any{"Identifier": "deep", "Name": "deep-proj"},
			},
		},
	}
	walkSessions(data, now, seen)
	if len(seen) != 1 {
		t.Errorf("expected 1 entry, got %d", len(seen))
	}
}
