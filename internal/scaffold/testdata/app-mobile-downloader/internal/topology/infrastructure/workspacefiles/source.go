package workspacefiles

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	topologyapp "gitinittest5/internal/topology/application"
)

type sessionFile struct {
	SessionID   string    `json:"session_id"`
	ProjectName string    `json:"project_name"`
	Email       string    `json:"email"`
	Hostname    string    `json:"hostname"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	Source      string    `json:"source"`
}

type Source struct {
	root string
	now  func() time.Time
	ttl  time.Duration
}

func NewSource(root string, ttl time.Duration) *Source {
	if strings.TrimSpace(root) == "" {
		root = ".einar/sessions"
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &Source{root: root, now: time.Now, ttl: ttl}
}

func (s *Source) ListSyncSessions(context.Context) ([]topologyapp.SyncSession, error) {
	if s == nil {
		return nil, nil
	}
	if s.now == nil {
		s.now = time.Now
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	now := s.now()
	result := make([]topologyapp.SyncSession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if err != nil {
			continue
		}
		var meta sessionFile
		if err := json.Unmarshal(b, &meta); err != nil {
			continue
		}
		if strings.TrimSpace(meta.SessionID) == "" || strings.TrimSpace(meta.ProjectName) == "" {
			continue
		}
		if meta.LastSeenAt.IsZero() {
			if info, err := entry.Info(); err == nil {
				meta.LastSeenAt = info.ModTime()
			}
		}
		if meta.LastSeenAt.IsZero() || now.Sub(meta.LastSeenAt) > s.ttl {
			continue
		}
		result = append(result, topologyapp.SyncSession{
			SessionID:   fileSessionID(strings.TrimSpace(meta.SessionID), strings.TrimSpace(meta.Hostname)),
			ProjectName: strings.TrimSpace(meta.ProjectName),
			ClientName:  clientName(meta.Email, meta.Hostname),
			Status:      topologyapp.StatusRunning,
			Source:      firstNonEmpty(strings.TrimSpace(meta.Source), "cli-file"),
			StartedAt:   meta.LastSeenAt,
			LastSeenAt:  meta.LastSeenAt,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectName == result[j].ProjectName {
			return result[i].SessionID < result[j].SessionID
		}
		return result[i].ProjectName < result[j].ProjectName
	})
	return result, nil
}

func clientName(email, hostname string) string {
	email = strings.TrimSpace(email)
	hostname = strings.TrimSpace(hostname)
	switch {
	case email == "":
		return hostname
	case hostname == "":
		return email
	default:
		return email + " · " + hostname
	}
}

func fileSessionID(sessionID, hostname string) string {
	sessionID = strings.TrimSpace(sessionID)
	hostname = strings.TrimSpace(hostname)
	if sessionID == "" {
		return hostname
	}
	if hostname == "" {
		return sessionID
	}
	return sessionID + "--" + hostname
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
