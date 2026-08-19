package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeProvider es un MemoryProvider in-memory que registra
// todas las llamadas. Sirve para testear la integración del
// Manager sin depender de Honcho real ni del adapter de
// infrastructure.
type fakeProvider struct {
	mu        sync.Mutex
	ensures   []MemoryKey
	recalls   []recallCall
	remembers []rememberCall
	// Hooks inyectables para forzar comportamiento.
	recallFn   func(ctx context.Context, key MemoryKey, query string) (MemoryContext, error)
	ensureFn   func(ctx context.Context, key MemoryKey) error
	rememberFn func(ctx context.Context, key MemoryKey, msgs []MemoryMessage) error
}

type recallCall struct {
	Key   MemoryKey
	Query string
}

type rememberCall struct {
	Key  MemoryKey
	Msgs []MemoryMessage
}

func (f *fakeProvider) EnsurePeers(ctx context.Context, key MemoryKey) error {
	f.mu.Lock()
	f.ensures = append(f.ensures, key)
	fn := f.ensureFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, key)
	}
	return nil
}

func (f *fakeProvider) Recall(ctx context.Context, key MemoryKey, query string) (MemoryContext, error) {
	f.mu.Lock()
	f.recalls = append(f.recalls, recallCall{Key: key, Query: query})
	fn := f.recallFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, key, query)
	}
	return MemoryContext{}, nil
}

func (f *fakeProvider) Remember(ctx context.Context, key MemoryKey, msgs []MemoryMessage) error {
	f.mu.Lock()
	f.remembers = append(f.remembers, rememberCall{Key: key, Msgs: append([]MemoryMessage(nil), msgs...)})
	fn := f.rememberFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, key, msgs)
	}
	return nil
}

func (f *fakeProvider) ensureCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ensures)
}

func (f *fakeProvider) recallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.recalls)
}

func (f *fakeProvider) rememberCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.remembers)
}

// Compile-time check: fakeProvider implementa MemoryProvider.
var _ MemoryProvider = (*fakeProvider)(nil)

// steerCount devuelve el total de llamadas a Steer sumando
// todos los runtimes creados por el factoryRunner. factoryRunner
// crea uno fresco por Start() (típico cuando el pool evicta
// entre prompts), así que el steer del primer runtime puede
// haber ocurrido en una instancia distinta a la del último.
func steerCount(r *factoryRunner) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, rt := range r.created {
		rt.mu.Lock()
		total += rt.steerCalls
		rt.mu.Unlock()
	}
	return total
}

func promptCount(r *factoryRunner) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	total := 0
	for _, rt := range r.created {
		rt.mu.Lock()
		total += rt.promptCalls
		rt.mu.Unlock()
	}
	return total
}

func TestPromptRequest_InjectsMemory(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{
		recallFn: func(ctx context.Context, key MemoryKey, query string) (MemoryContext, error) {
			return MemoryContext{Text: "user prefers Go"}, nil
		},
	}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title:      "test",
		CWD:        t.TempDir(),
		Model:      "test-model",
		OwnerEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := manager.PromptRequest(ctx, session.ID, PromptInput{
		Message:   "hola",
		TurnID:    "turn-1",
		UserEmail: "alice@example.com",
	}); err != nil {
		t.Fatalf("PromptRequest: %v", err)
	}
	if got := provider.ensureCount(); got != 1 {
		t.Errorf("EnsurePeers calls: got %d, want 1", got)
	}
	if got := provider.recallCount(); got != 1 {
		t.Errorf("Recall calls: got %d, want 1", got)
	}
	// Sólo 1 memory recall steering (default está desactivado desde
	// el fix de LLM coherence; ver manager.go:522).
	if got := steerCount(runner); got != 1 {
		t.Errorf("Steer calls: got %d, want 1 (only memory)", got)
	}
}

func TestPromptRequest_NoopProvider_DoesNotInject(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	// Sin WithMemory: usa el noopProvider default.
	manager := NewManager(store, runner).WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title:      "test",
		CWD:        t.TempDir(),
		OwnerEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := manager.PromptRequest(ctx, session.ID, PromptInput{
		Message:   "hola",
		TurnID:    "turn-1",
		UserEmail: "alice@example.com",
	}); err != nil {
		t.Fatalf("PromptRequest: %v", err)
	}
	// Sin default steer, sin memory steer: 0 steers totales.
	if got := steerCount(runner); got != 0 {
		t.Errorf("Steer calls with noop and no default: got %d, want 0", got)
	}
}

func TestPromptRequest_SlotMarker_PreventsReinject(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{
		recallFn: func(ctx context.Context, key MemoryKey, query string) (MemoryContext, error) {
			return MemoryContext{Text: "context"}, nil
		},
	}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title:      "test",
		CWD:        t.TempDir(),
		OwnerEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Mismo TurnID dos veces. Primer PromptRequest: 1 memory
	// steer (default desactivado). Segundo: el slot marker
	// evita el memory steer => sigue en 1.
	for i := 0; i < 2; i++ {
		// Reseteamos el status a idle entre prompts porque el
		// stubRuntime no marca el final del turno por sí solo;
		// en producción lo hace el evento turn_end/agent_end.
		stored, _ := store.Get(ctx, session.ID)
		stored.Status = SessionStatusIdle
		_ = store.Update(ctx, stored)
		if err := manager.PromptRequest(ctx, session.ID, PromptInput{
			Message:   "msg",
			TurnID:    "turn-A",
			UserEmail: "alice@example.com",
		}); err != nil {
			t.Fatalf("PromptRequest[%d]: %v", i, err)
		}
	}
	if got := steerCount(runner); got != 1 {
		t.Errorf("Steer calls same TurnID: got %d, want 1 (memory once, default disabled)", got)
	}
	// TurnID distinto: vuelve a steerear memory, total = 2.
	stored, _ := store.Get(ctx, session.ID)
	stored.Status = SessionStatusIdle
	_ = store.Update(ctx, stored)
	if err := manager.PromptRequest(ctx, session.ID, PromptInput{
		Message:   "msg",
		TurnID:    "turn-B",
		UserEmail: "alice@example.com",
	}); err != nil {
		t.Fatalf("PromptRequest[turn-B]: %v", err)
	}
	if got := steerCount(runner); got != 2 {
		t.Errorf("Steer calls different TurnID: got %d, want 2 (memory twice, default disabled)", got)
	}
}

func TestPromptRequest_EmptyUserEmail_SkipsMemory(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title: "test",
		CWD:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := manager.PromptRequest(ctx, session.ID, PromptInput{
		Message: "hola",
		TurnID:  "turn-1",
	}); err != nil {
		t.Fatalf("PromptRequest: %v", err)
	}
	if got := provider.ensureCount(); got != 0 {
		t.Errorf("EnsurePeers should not be called when UserEmail empty, got %d", got)
	}
	if got := provider.recallCount(); got != 0 {
		t.Errorf("Recall should not be called when UserEmail empty, got %d", got)
	}
}

func TestPromptRequest_ProviderError_DoesNotBlockPrompt(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{
		ensureFn: func(ctx context.Context, key MemoryKey) error {
			return errors.New("honcho: 503")
		},
		recallFn: func(ctx context.Context, key MemoryKey, query string) (MemoryContext, error) {
			return MemoryContext{}, errors.New("honcho: 503")
		},
	}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title:      "test",
		CWD:        t.TempDir(),
		OwnerEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := manager.PromptRequest(ctx, session.ID, PromptInput{
		Message:   "hola",
		TurnID:    "turn-1",
		UserEmail: "alice@example.com",
	}); err != nil {
		t.Fatalf("PromptRequest should NOT fail when provider errors: %v", err)
	}
	if got := promptCount(runner); got != 1 {
		t.Errorf("Runner.Prompt calls: got %d, want 1", got)
	}
}

func TestFlushMemoryRemember_FlushesAfterAgentEnd(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title:      "test",
		CWD:        t.TempDir(),
		OwnerEmail: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Materializamos un user prompt al transcript.
	MaterializeUserPrompt(session.ID, "user dijo hola")
	// Simulamos un agent_end.
	manager.broadcast(session.ID, Event{Type: "agent_end", Payload: []byte(`{}`)})
	// El flush es en goroutine; esperamos a que termine.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if provider.rememberCount() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := provider.rememberCount(); got == 0 {
		t.Fatal("Remember was not called after agent_end")
	}
}

func TestFlushMemoryRemember_NoopWhenNoOwnerEmail(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	provider := &fakeProvider{}
	manager := NewManager(store, runner).
		WithMemory(provider).
		WithMemoryWorkspace("ws-test")
	ctx := context.Background()
	session, err := manager.Create(ctx, CreateSessionInput{
		Title: "test",
		CWD:   t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	MaterializeUserPrompt(session.ID, "user dijo hola")
	manager.broadcast(session.ID, Event{Type: "agent_end", Payload: []byte(`{}`)})
	time.Sleep(100 * time.Millisecond)
	if got := provider.rememberCount(); got != 0 {
		t.Errorf("Remember should be no-op without OwnerEmail, got %d calls", got)
	}
}
