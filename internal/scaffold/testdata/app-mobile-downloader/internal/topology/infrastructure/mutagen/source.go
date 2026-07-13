package mutagen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	topologyapp "fixtests1/internal/topology/application"
)

type Source struct {
	lookPath func(string) (string, error)
	run      func(context.Context, string, ...string) ([]byte, error)
	now      func() time.Time
}

func NewSource() *Source {
	return &Source{
		lookPath: exec.LookPath,
		run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			return cmd.Output()
		},
		now: time.Now,
	}
}

func (s *Source) ListSyncSessions(ctx context.Context) ([]topologyapp.SyncSession, error) {
	if s == nil {
		return nil, nil
	}
	if s.lookPath == nil {
		s.lookPath = exec.LookPath
	}
	if s.run == nil {
		s.run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			cmd := exec.CommandContext(ctx, name, args...)
			return cmd.Output()
		}
	}
	if s.now == nil {
		s.now = time.Now
	}
	mutagenPath, err := resolveMutagenPath(s.lookPath)
	if err != nil {
		return nil, nil
	}

	commands := [][]string{
		{"sync", "list", "--output", "json"},
		{"sync", "list", "-o", "json"},
	}
	for _, args := range commands {
		output, err := s.run(ctx, mutagenPath, args...)
		if err != nil || len(bytes.TrimSpace(output)) == 0 {
			continue
		}
		sessions, err := parseSyncSessions(output, s.now())
		if err == nil {
			return sessions, nil
		}
	}
	return nil, nil
}

func parseSyncSessions(data []byte, now time.Time) ([]topologyapp.SyncSession, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	seen := map[string]topologyapp.SyncSession{}
	walkSessions(raw, now, seen)
	if len(seen) == 0 {
		return nil, errors.New("no sync sessions found")
	}

	sessions := make([]topologyapp.SyncSession, 0, len(seen))
	for _, session := range seen {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].ProjectName == sessions[j].ProjectName {
			return sessions[i].SessionID < sessions[j].SessionID
		}
		return sessions[i].ProjectName < sessions[j].ProjectName
	})
	return sessions, nil
}

func walkSessions(value any, now time.Time, seen map[string]topologyapp.SyncSession) {
	switch typed := value.(type) {
	case map[string]any:
		if session, ok := sessionFromMap(typed, now); ok {
			key := session.SessionID
			if key == "" {
				key = session.ProjectName
			}
			if key != "" {
				seen[key] = session
			}
		}
		for _, nested := range typed {
			walkSessions(nested, now, seen)
		}
	case []any:
		for _, nested := range typed {
			walkSessions(nested, now, seen)
		}
	}
}

func sessionFromMap(m map[string]any, now time.Time) (topologyapp.SyncSession, bool) {
	id := firstString(m, "identifier", "Identifier", "session", "Session")
	name := firstString(m, "name", "Name")
	status := deriveStatus(m)

	if id == "" && name == "" {
		return topologyapp.SyncSession{}, false
	}
	if status == "" {
		status = topologyapp.StatusRunning
	}
	if name == "" {
		name = id
	}

	return topologyapp.SyncSession{
		SessionID:   id,
		ProjectName: name,
		Status:      status,
		Source:      "mutagen",
		LastSeenAt:  now,
	}, true
}

func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if text, ok := value.(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func resolveMutagenPath(lookPath func(string) (string, error)) (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("MUTAGEN_PATH")); explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
	}
	if lookPath != nil {
		if path, err := lookPath(binaryName("mutagen")); err == nil {
			return path, nil
		}
		if path, err := lookPath("mutagen"); err == nil {
			return path, nil
		}
	}
	for _, candidate := range localCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", errors.New("mutagen not found")
}

func localCandidates() []string {
	var candidates []string
	name := binaryName("mutagen")
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), name))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			filepath.Join(wd, name),
			filepath.Join(wd, "bin", name),
		)
	}
	return candidates
}

func binaryName(name string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

func deriveStatus(m map[string]any) string {
	if paused, ok := m["Paused"].(bool); ok && paused {
		return topologyapp.StatusOffline
	}
	if paused, ok := m["paused"].(bool); ok && paused {
		return topologyapp.StatusOffline
	}
	candidate := strings.ToLower(firstString(m, "status", "Status", "state", "State"))
	switch {
	case candidate == "":
		return ""
	case strings.Contains(candidate, "sync") || strings.Contains(candidate, "scan") || strings.Contains(candidate, "stage"):
		return topologyapp.StatusSyncing
	case strings.Contains(candidate, "pause") || strings.Contains(candidate, "halt") || strings.Contains(candidate, "offline"):
		return topologyapp.StatusOffline
	case strings.Contains(candidate, "problem") || strings.Contains(candidate, "error") || strings.Contains(candidate, "fail") || strings.Contains(candidate, "stalled") || strings.Contains(candidate, "degrad"):
		return topologyapp.StatusDegraded
	default:
		return topologyapp.StatusRunning
	}
}
