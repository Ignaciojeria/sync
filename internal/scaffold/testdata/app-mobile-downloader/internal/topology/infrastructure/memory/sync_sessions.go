package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	topologyapp "app-mobile-downloader/internal/topology/application"
)

type SyncSessionsStore struct {
	mu       sync.Mutex
	sessions map[string]topologyapp.SyncSession
	ttl      time.Duration
	now      func() time.Time
}

func NewSyncSessionsStore(ttl time.Duration) *SyncSessionsStore {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &SyncSessionsStore{
		sessions: map[string]topologyapp.SyncSession{},
		ttl:      ttl,
		now:      time.Now,
	}
}

func (s *SyncSessionsStore) UpsertSyncSession(_ context.Context, session topologyapp.SyncSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session.LastSeenAt.IsZero() {
		session.LastSeenAt = s.now()
	}
	if existing, ok := s.sessions[session.SessionID]; ok && existing.StartedAt.IsZero() {
		session.StartedAt = existing.StartedAt
	}
	if session.StartedAt.IsZero() {
		session.StartedAt = session.LastSeenAt
	}
	s.sessions[session.SessionID] = session
	return nil
}

func (s *SyncSessionsStore) ListSyncSessions(_ context.Context) ([]topologyapp.SyncSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	result := make([]topologyapp.SyncSession, 0, len(s.sessions))
	for key, session := range s.sessions {
		if session.Status == topologyapp.StatusOffline || now.Sub(session.LastSeenAt) > s.ttl {
			delete(s.sessions, key)
			continue
		}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ProjectName == result[j].ProjectName {
			return result[i].SessionID < result[j].SessionID
		}
		return result[i].ProjectName < result[j].ProjectName
	})
	return result, nil
}
