package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Ignaciojeria/sync/internal/config"
)

func TestTopologySessionIDStable(t *testing.T) {
	path := filepath.Clean("/tmp/project")
	a := topologySessionID(path, "workspace-gateway")
	b := topologySessionID(path, "workspace-gateway")
	if a != b {
		t.Fatalf("session ids differ: %q != %q", a, b)
	}
}

func TestTopologyHeartbeatMonitorWritesSessionFile(t *testing.T) {
	dir := t.TempDir()
	monitor := newTopologyHeartbeatMonitor(config.Config{Token: unsignedJWTForHeartbeatTest(t, map[string]any{"email": "user@example.com"})}, topologyHeartbeatOptions{
		ProjectPath: dir,
		ProjectName: "workspace-gateway",
		SessionID:   "sync-1",
		ClientName:  "test-client",
		Source:      "mutagen",
		Interval:    time.Millisecond,
	})

	if err := monitor.heartbeatIfProjectExists(context.Background()); err != nil {
		t.Fatalf("heartbeatIfProjectExists() error = %v", err)
	}
	path := filepath.Join(dir, ".einar", "sessions", topologySessionFileName("sync-1", localClientName())+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if payload["project_name"] != "workspace-gateway" {
		t.Fatalf("project_name = %v", payload["project_name"])
	}
	if payload["source"] != "cli-file" {
		t.Fatalf("source = %v", payload["source"])
	}
}

func TestTopologyHeartbeatMonitorMissingProjectDoesNothing(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	monitor := newTopologyHeartbeatMonitor(config.Config{}, topologyHeartbeatOptions{
		ProjectPath: missing,
		ProjectName: "workspace-gateway",
		SessionID:   "sync-1",
		ClientName:  "test-client",
		Source:      "mutagen",
	})

	if err := monitor.heartbeatIfProjectExists(context.Background()); err != nil {
		t.Fatalf("heartbeatIfProjectExists() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(missing, ".einar", "sessions", topologySessionFileName("sync-1", localClientName())+".json")); !os.IsNotExist(err) {
		t.Fatalf("expected no session file, err=%v", err)
	}
}

func TestLocalClientNameNotEmpty(t *testing.T) {
	if got := localClientName(); got == "" {
		t.Fatal("expected client name")
	}
	_, _ = os.Hostname()
}

func TestPrepareHeartbeatRunner(t *testing.T) {
	source := filepath.Join(t.TempDir(), "sync.exe")
	if err := os.WriteFile(source, []byte("hello"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner, err := prepareHeartbeatRunner(source)
	if err != nil {
		t.Fatalf("prepareHeartbeatRunner() error = %v", err)
	}
	if runtime.GOOS == "windows" && runner == source {
		t.Fatal("expected copied runner on windows")
	}
	if _, err := os.Stat(runner); err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
}

func TestHeartbeatPIDFileLifecycle(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), heartbeatPIDFileName)
	if err := writeHeartbeatPIDFile(pidFile, os.Getpid()); err != nil {
		t.Fatalf("writeHeartbeatPIDFile() error = %v", err)
	}
	alive, err := heartbeatProcessAlive(pidFile)
	if err != nil {
		t.Fatalf("heartbeatProcessAlive() error = %v", err)
	}
	if !alive {
		t.Fatal("expected alive heartbeat process")
	}
	removeHeartbeatPIDFile(pidFile, os.Getpid())
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected pid file removed, err=%v", err)
	}
}

func unsignedJWTForHeartbeatTest(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]any{"alg": "none", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload) + "."
}

func TestHeartbeatProcessAliveRemovesStalePID(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), heartbeatPIDFileName)
	if err := writeHeartbeatPIDFile(pidFile, 999999); err != nil {
		t.Fatalf("writeHeartbeatPIDFile() error = %v", err)
	}
	alive, err := heartbeatProcessAlive(pidFile)
	if err != nil {
		t.Fatalf("heartbeatProcessAlive() error = %v", err)
	}
	if alive {
		t.Fatal("expected stale heartbeat pid to be false")
	}
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Fatalf("expected stale pid file removed, err=%v", err)
	}
}
