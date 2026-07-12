package preview

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	agentapp "testboi1/pkg/agent/application"
)

// Launcher levanta una preview HTTP por sesión usando el workspace aislado.
// ponytail: acoplado a este repo (cmd/api + .air.toml). Si el boilerplate
// cambia, recién ahí conviene volverlo configurable.
type Launcher struct {
	mu        sync.Mutex
	processes map[string]*processEntry
}

type processEntry struct {
	cmd     *exec.Cmd
	logFile *os.File
	mode    string
}

func NewLauncher() *Launcher {
	return &Launcher{processes: map[string]*processEntry{}}
}

func (l *Launcher) Prepare(ctx context.Context, session agentapp.Session) (agentapp.Session, error) {
	workspace := strings.TrimSpace(session.WorkspacePath)
	if workspace == "" {
		workspace = strings.TrimSpace(session.CWD)
	}
	if workspace == "" {
		return session, nil
	}
	if _, err := os.Stat(filepath.Join(workspace, "cmd", "api")); err != nil {
		return session, nil
	}
	port, err := pickFreePort()
	if err != nil {
		return session, fmt.Errorf("preview: pick port: %w", err)
	}
	entry, err := startWorkspaceProcess(workspace, port)
	if err != nil {
		return session, fmt.Errorf("preview: start workspace process: %w", err)
	}
	if err := waitUntilHTTPReady(ctx, port); err != nil {
		_ = stopEntry(entry)
		return session, fmt.Errorf("preview: wait for port %d: %w", port, err)
	}

	l.mu.Lock()
	l.processes[session.ID] = entry
	l.mu.Unlock()

	session.PreviewPort = port
	session.PreviewHealth = "/"
	session.PreviewStatus = agentapp.PreviewStatusLive
	session.PreviewURL = "/agent/sessions/" + session.ID + "/preview/"
	slog.Info("agent preview ready",
		"session_id", session.ID,
		"port", port,
		"workspace", workspace,
		"mode", entry.mode,
	)
	return session, nil
}

func (l *Launcher) Destroy(_ context.Context, session agentapp.Session) error {
	l.mu.Lock()
	entry := l.processes[session.ID]
	delete(l.processes, session.ID)
	l.mu.Unlock()
	return stopEntry(entry)
}

func startWorkspaceProcess(workspace string, port int) (*processEntry, error) {
	logPath := filepath.Join(workspace, "tmp", "run", "agent-preview.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	cmd, mode, err := previewCommand(workspace)
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	cmd.Dir = workspace
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		"AGENT_SESSION_DIR="+filepath.Join(workspace, "tmp", "agent-sessions"),
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}
	return &processEntry{cmd: cmd, logFile: logFile, mode: mode}, nil
}

func previewCommand(workspace string) (*exec.Cmd, string, error) {
	airConfig := filepath.Join(workspace, ".air.toml")
	if info, err := os.Stat(airConfig); err == nil && !info.IsDir() {
		if airPath, err := exec.LookPath("air"); err == nil {
			return exec.Command(airPath, "-c", airConfig), "air", nil
		}
		if _, err := exec.LookPath("go"); err == nil {
			return exec.Command("go", "run", "github.com/air-verse/air@latest", "-c", airConfig), "air-go-run", nil
		}
	}
	if err := buildWorkspaceBinary(context.Background(), workspace); err != nil {
		return nil, "", err
	}
	return exec.Command(previewBinaryPath(workspace)), "binary", nil
}

func buildWorkspaceBinary(ctx context.Context, workspace string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := generateTempl(ctx, workspace); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", previewBinaryPath(workspace), "./cmd/api")
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func generateTempl(ctx context.Context, workspace string) error {
	if templPath, err := exec.LookPath("templ"); err == nil {
		cmd := exec.CommandContext(ctx, templPath, "generate")
		cmd.Dir = workspace
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("templ generate: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		templPath := filepath.Join(home, "go", "bin", "templ")
		if info, err := os.Stat(templPath); err == nil && !info.IsDir() {
			cmd := exec.CommandContext(ctx, templPath, "generate")
			cmd.Dir = workspace
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("templ generate: %w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	cmd := exec.CommandContext(ctx, "go", "run", "github.com/a-h/templ/cmd/templ@v0.3.1020", "generate")
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("templ generate: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func previewBinaryPath(workspace string) string {
	name := "preview-main"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(workspace, "tmp", name)
}

func stopEntry(entry *processEntry) error {
	if entry == nil {
		return nil
	}
	defer func() {
		if entry.logFile != nil {
			_ = entry.logFile.Close()
		}
	}()
	if entry.cmd == nil || entry.cmd.Process == nil {
		return nil
	}
	if err := entry.cmd.Process.Kill(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "finished") {
		return err
	}
	_ = entry.cmd.Wait()
	return nil
}

func pickFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected addr type %T", ln.Addr())
	}
	return tcp.Port, nil
}

func waitUntilHTTPReady(ctx context.Context, port int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(45 * time.Second)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for preview")
		}
		_, err := agentapp.HealthcheckPreview(port, "/")
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
}
