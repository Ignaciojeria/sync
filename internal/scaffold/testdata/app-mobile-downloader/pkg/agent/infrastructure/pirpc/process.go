package pirpc

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentapp "testboi1/pkg/agent/application"
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

	cwd, err := resolveCWD(spec.CWD, spec.SessionID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, extPath := range discoverProjectExtensions(cwd) {
		args = append(args, "-e", extPath)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	if len(args) > 2 {
		slog.Info("pirpc: starting pi",
			"session_id", spec.SessionID,
			"cwd", cwd,
			"args", args,
		)
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

func discoverProjectExtensions(cwd string) []string {
	roots := []string{filepath.Join(cwd, PiConfigDir, "extensions")}
	if rootCWD, err := os.Getwd(); err == nil {
		fallback := filepath.Join(rootCWD, PiConfigDir, "extensions")
		if !samePath(fallback, roots[0]) {
			roots = append(roots, fallback)
		}
	}

	seen := map[string]struct{}{}
	paths := make([]string, 0, 8)
	for _, extDir := range roots {
		entries, err := os.ReadDir(extDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			fullPath := filepath.Join(extDir, name)
			if entry.IsDir() {
				for _, candidate := range []string{filepath.Join(fullPath, "index.ts"), filepath.Join(fullPath, "index.js")} {
					if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
						if _, ok := seen[candidate]; !ok {
							seen[candidate] = struct{}{}
							paths = append(paths, candidate)
						}
						break
					}
				}
				continue
			}
			if strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js") {
				if _, ok := seen[fullPath]; ok {
					continue
				}
				seen[fullPath] = struct{}{}
				paths = append(paths, fullPath)
			}
		}
	}
	return paths
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == b {
		return true
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return false
}
