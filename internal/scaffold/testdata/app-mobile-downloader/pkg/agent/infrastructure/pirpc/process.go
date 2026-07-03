package pirpc

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentapp "app-mobile-downloader/pkg/agent/application"
)

func startProcess(ctx context.Context, binary string, spec agentapp.StartSpec) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	args := []string{"--mode", "rpc"}
	if sessionFile := strings.TrimSpace(spec.SessionFile); sessionFile != "" {
		if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
			return nil, nil, nil, nil, err
		}
		args = append(args, "--session", sessionFile)
	}
	if title := strings.TrimSpace(spec.Title); title != "" {
		args = append(args, "--name", title)
	}
	if model := strings.TrimSpace(spec.Model); model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, binary, args...)

	cwd, err := resolveCWD(spec.CWD, spec.SessionID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	cmd.Dir = cwd
	if cwd != strings.TrimSpace(spec.CWD) {
		slog.Info("pirpc: pi ejecutado en sandbox",
			"session_id", spec.SessionID,
			"cwd", cwd,
		)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, stdin, stdout, stderr, nil
}
