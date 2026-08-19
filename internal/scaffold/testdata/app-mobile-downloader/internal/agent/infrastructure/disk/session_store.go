// Package disk persiste metadata de sesiones del agente en disco para
// sobrevivir a restarts del proceso. Una sesión = un archivo JSON en <dir>/<id>.json.
package disk

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	agentapp "lastmile-agents/internal/agent/application"
)

// SessionStore implementa application.SessionStore con persistencia en disco.
// Mantiene una copia en memoria (write-through) para que List/Get sean rápidos,
// y reescribe el archivo en cada Create/Update con escritura atómica.
type SessionStore struct {
	dir string

	mu       sync.RWMutex
	sessions map[string]agentapp.Session
}

// NewSessionStore crea el store, asegura el directorio dir y carga las
// sesiones existentes desde archivos <id>.json válidos. Los archivos con
// JSON corrupto o sin extensión .json se ignoran.
func NewSessionStore(dir string) (*SessionStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("disk session store: dir is empty")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("disk session store: mkdir %q: %w", dir, err)
	}
	store := &SessionStore{
		dir:      dir,
		sessions: map[string]agentapp.Session{},
	}
	if err := store.loadFromDisk(); err != nil {
		return nil, err
	}
	return store, nil
}

// Dir devuelve la ruta usada por el store. Útil para tests y diagnóstico.
func (s *SessionStore) Dir() string { return s.dir }

func (s *SessionStore) loadFromDisk() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return fmt.Errorf("disk session store: read dir %q: %w", s.dir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		path := filepath.Join(s.dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			// No podemos generar error fatal: podría ser un archivo transitorio
			// dejado por una escritura atómica. Lo saltamos y seguimos.
			continue
		}
		var session agentapp.Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}
		if strings.TrimSpace(session.ID) == "" {
			continue
		}
		s.sessions[session.ID] = session
	}
	return nil
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
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("disk session store: session id is empty")
	}
	if err := s.writeAtomic(session); err != nil {
		return err
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
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
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("disk session store: session id is empty")
	}
	s.mu.RLock()
	_, exists := s.sessions[session.ID]
	s.mu.RUnlock()
	if !exists {
		return agentapp.ErrSessionNotFound
	}
	if err := s.writeAtomic(session); err != nil {
		return err
	}
	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()
	return nil
}

func (s *SessionStore) Delete(_ context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return agentapp.ErrSessionNotFound
	}
	s.mu.RLock()
	_, exists := s.sessions[id]
	s.mu.RUnlock()
	if !exists {
		return agentapp.ErrSessionNotFound
	}
	finalPath := filepath.Join(s.dir, id+".json")
	if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("disk session store: remove %q: %w", finalPath, err)
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
	return nil
}

// writeAtomic serializa session y la escribe vía archivo temporal + rename,
// para evitar archivos truncados si el proceso muere a mitad de una escritura.
func (s *SessionStore) writeAtomic(session agentapp.Session) error {
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("disk session store: marshal %q: %w", session.ID, err)
	}
	finalPath := filepath.Join(s.dir, session.ID+".json")
	tmpPath := filepath.Join(s.dir, session.ID+".json.tmp")

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("disk session store: write %q: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("disk session store: rename %q -> %q: %w", tmpPath, finalPath, err)
	}
	return nil
}
