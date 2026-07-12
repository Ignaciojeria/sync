package memory

import (
	"context"
	"sort"
	"sync"

	agentapp "testboi1/pkg/agent/application"
)

// SessionStore persiste metadata de sesiones en memoria para el MVP.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]agentapp.Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: map[string]agentapp.Session{}}
}

func (s *SessionStore) List(_ context.Context) ([]agentapp.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]agentapp.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		out = append(out, session)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *SessionStore) Create(_ context.Context, session agentapp.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStore) Get(_ context.Context, id string) (agentapp.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return agentapp.Session{}, agentapp.ErrSessionNotFound
	}
	return session, nil
}

func (s *SessionStore) Update(_ context.Context, session agentapp.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[session.ID]; !ok {
		return agentapp.ErrSessionNotFound
	}
	s.sessions[session.ID] = session
	return nil
}

func (s *SessionStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return agentapp.ErrSessionNotFound
	}
	delete(s.sessions, id)
	return nil
}
