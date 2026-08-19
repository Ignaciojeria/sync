package application

import (
	"context"
	"testing"
)

// memStoreStub es un SessionStore mínimo en memoria para los
// tests de Create/AgentID. No necesitamos persistencia ni features
// de disk store — sólo Get/Create/Update para que el Manager no
// falle en sus backfills.
type memStoreStub struct {
	sessions map[string]Session
}

func newMemStoreStub() *memStoreStub {
	return &memStoreStub{sessions: map[string]Session{}}
}

func (s *memStoreStub) List(_ context.Context) ([]Session, error) {
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out, nil
}

func (s *memStoreStub) Create(_ context.Context, sess Session) error {
	s.sessions[sess.ID] = sess
	return nil
}

func (s *memStoreStub) Get(_ context.Context, id string) (Session, error) {
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

func (s *memStoreStub) Update(_ context.Context, sess Session) error {
	s.sessions[sess.ID] = sess
	return nil
}

func (s *memStoreStub) Delete(_ context.Context, id string) error {
	delete(s.sessions, id)
	return nil
}

// runnerStub es un Runner no-op para que ensureRuntime no haga
// spawn real. Sólo queremos ejercitar Create + ensureSessionDefaults.
type runnerStub struct{}

func (runnerStub) Start(_ context.Context, _ StartSpec) (Runtime, error) {
	return nil, nil
}

func TestDefaultAgentID(t *testing.T) {
	got := DefaultAgentID()
	if got != "develop" {
		t.Fatalf("DefaultAgentID() = %q, want %q", got, "develop")
	}
}

func TestResolveAgentID(t *testing.T) {
	cases := []struct {
		in        string
		wantID    string
		wantValid bool
	}{
		{"", "develop", false},      // vacío cae al default
		{"  ", "develop", false},    // whitespace también
		{"develop", "develop", true},
		{"unknown", "develop", false}, // desconocido cae al default
	}
	for _, c := range cases {
		got, valid := ResolveAgentID(c.in)
		if got != c.wantID || valid != c.wantValid {
			t.Errorf("ResolveAgentID(%q) = (%q, %v), want (%q, %v)", c.in, got, valid, c.wantID, c.wantValid)
		}
	}
}

func TestCreate_DefaultsAgentIDToDevelop(t *testing.T) {
	store := newMemStoreStub()
	mgr := NewManager(store, runnerStub{})

	sess, err := mgr.Create(context.Background(), CreateSessionInput{
		Title: "Sin agentID",
		CWD:   ".",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.AgentID != "develop" {
		t.Fatalf("AgentID persistido = %q, want %q", sess.AgentID, "develop")
	}
}

func TestCreate_RespectsExplicitAgentID(t *testing.T) {
	store := newMemStoreStub()
	mgr := NewManager(store, runnerStub{})

	sess, err := mgr.Create(context.Background(), CreateSessionInput{
		Title:   "Con agentID",
		CWD:     ".",
		AgentID: "develop",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.AgentID != "develop" {
		t.Fatalf("AgentID persistido = %q, want %q", sess.AgentID, "develop")
	}
}

func TestCreate_UnknownAgentIDFallsBackToDefault(t *testing.T) {
	store := newMemStoreStub()
	mgr := NewManager(store, runnerStub{})

	// "reviewer" no está en el registry todavía. El Manager debe
	// normalizar al default (develop) en vez de fallar o persistir
	// un ID desconocido.
	sess, err := mgr.Create(context.Background(), CreateSessionInput{
		Title:   "Con agentID desconocido",
		CWD:     ".",
		AgentID: "reviewer",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.AgentID != "develop" {
		t.Fatalf("AgentID persistido = %q, want %q (fallback al default)", sess.AgentID, "develop")
	}
}

func TestEnsureSessionDefaults_BackfillsAgentID(t *testing.T) {
	// Sesión "legacy" creada antes del multi-agente: tiene todo
	// menos AgentID. ensureSessionDefaults debe backfillearlo para
	// que el runner no reciba un AgentID vacío cuando se spawn.
	store := newMemStoreStub()
	legacy := Session{
		ID:    "legacy-1",
		Title: "Vieja",
		CWD:   ".",
	}
	store.sessions[legacy.ID] = legacy

	mgr := NewManager(store, runnerStub{})
	got, err := mgr.Get(context.Background(), legacy.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AgentID != "develop" {
		t.Fatalf("AgentID tras Get = %q, want %q (backfill)", got.AgentID, "develop")
	}
	// El backfill debe haberse persistido.
	persisted, _ := store.Get(context.Background(), legacy.ID)
	if persisted.AgentID != "develop" {
		t.Fatalf("AgentID persistido = %q, want %q", persisted.AgentID, "develop")
	}
}
