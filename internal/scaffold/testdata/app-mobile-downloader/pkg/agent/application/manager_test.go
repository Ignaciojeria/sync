package application

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// --- session store stub ---

type stubStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func newStubStore() *stubStore {
	return &stubStore{sessions: map[string]Session{}}
}

func (s *stubStore) List(ctx context.Context) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out, nil
}

func (s *stubStore) Create(ctx context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *stubStore) Get(ctx context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return sess, nil
}

func (s *stubStore) Update(ctx context.Context, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[session.ID]
	if !ok {
		return ErrSessionNotFound
	}
	s.sessions[session.ID] = session
	return nil
}

// --- runtime stub ---

type stubRuntime struct {
	mu sync.Mutex

	started         bool
	closed          bool
	promptCalls     int
	promptErr       error
	promptHook      func(ctx context.Context, msg string) error
	subscribeCh     chan Event
	subscribeCancel func()
}

func (r *stubRuntime) markClosed() {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
}

func (r *stubRuntime) SessionID() string { return "" }

func (r *stubRuntime) Prompt(ctx context.Context, message string) error {
	r.mu.Lock()
	r.promptCalls++
	hook := r.promptHook
	r.mu.Unlock()
	if hook != nil {
		return hook(ctx, message)
	}
	return r.promptErr
}

func (r *stubRuntime) Steer(ctx context.Context, _ string) error { return nil }
func (r *stubRuntime) Abort(ctx context.Context) error           { return nil }

func (r *stubRuntime) Subscribe() (<-chan Event, func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.subscribeCh == nil {
		r.subscribeCh = make(chan Event, 8)
	}
	cancel := func() {
		r.mu.Lock()
		r.subscribeCancel = nil
		r.mu.Unlock()
	}
	r.subscribeCancel = cancel
	return r.subscribeCh, cancel
}

func (r *stubRuntime) State() RuntimeState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RuntimeState{Closed: r.closed}
}

func (r *stubRuntime) Close() error {
	r.mu.Lock()
	r.closed = true
	ch := r.subscribeCh
	r.subscribeCh = nil
	r.subscribeCancel = nil
	r.mu.Unlock()
	if ch != nil {
		close(ch)
	}
	return nil
}

// --- factory runner: produce un Runtime fresco por cada Start() ---

type factoryRunner struct {
	mu       sync.Mutex
	created  []*stubRuntime
	lastSpec StartSpec
}

func (f *factoryRunner) Start(_ context.Context, spec StartSpec) (Runtime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSpec = spec
	rt := &stubRuntime{started: true}
	f.created = append(f.created, rt)
	return rt, nil
}

func (f *factoryRunner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func (f *factoryRunner) last() *stubRuntime {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.created) == 0 {
		return nil
	}
	return f.created[len(f.created)-1]
}

func (f *factoryRunner) spec() StartSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastSpec
}

// --- tests ---

func TestNewManager_WiresDeps(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)
	if m == nil {
		t.Fatal("manager nil")
	}
	if m.store != store {
		t.Fatal("store no wireado")
	}
	if m.runner != runner {
		t.Fatal("runner no wireado")
	}
}

func TestManagerEnsure_StartsOnce(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure #1: %v", err)
	}
	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure #2: %v", err)
	}
	if got, want := runner.count(), 1; got != want {
		t.Fatalf("Start llamado %d veces, want %d", got, want)
	}
	if got, want := runner.spec().SessionFile, sess.PiSessionFile; got != want {
		t.Fatalf("SessionFile = %q, want %q", got, want)
	}
}

func TestManagerEnsure_RespawnsAfterClosedRuntime(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure #1: %v", err)
	}

	// Cerramos la runtime para forzar un spawn nuevo en el próximo Ensure.
	rt := runner.last()
	if rt == nil {
		t.Fatal("factory no produjo runtime")
	}
	rt.markClosed()

	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure #2: %v", err)
	}
	if got, want := runner.count(), 2; got != want {
		t.Fatalf("Start llamado %d veces tras cierre, want %d", got, want)
	}
}

func TestManagerEnsure_ReturnsErrSessionNotFound(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	err := m.Ensure(context.Background(), "missing-id")
	if err != ErrSessionNotFound {
		t.Fatalf("err = %v, want ErrSessionNotFound", err)
	}
	if got := runner.count(); got != 0 {
		t.Fatalf("Start llamado %d veces para id inexistente", got)
	}
}

func TestManagerSubscribe_DoesNotStartRuntime(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, cancel, err := m.Subscribe(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	if ch == nil {
		t.Fatal("channel nil")
	}
	if got := runner.count(); got != 0 {
		t.Fatalf("Start llamado %d veces al suscribirse, want 0", got)
	}
}

func TestManagerSubscribe_ReceivesRuntimeEventsAfterPromptSpawn(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch, cancel, err := m.Subscribe(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()

	if err := m.Prompt(ctx, sess.ID, "hola"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	rt := runner.last()
	if rt == nil || rt.subscribeCh == nil {
		t.Fatal("runtime sin canal de eventos")
	}
	want := Event{SessionID: sess.ID, Type: "message_start"}
	rt.subscribeCh <- want

	select {
	case got := <-ch:
		if got.Type != want.Type || got.SessionID != want.SessionID {
			t.Fatalf("event = %+v, want %+v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout esperando evento")
	}
}

func TestManagerPrompt_AppliesTimeoutAndFailsFast(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Forzamos promptCallTimeout a un valor chico para que el test no tarde.
	orig := promptCallTimeout
	promptCallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { promptCallTimeout = orig })

	// El stub de runtime bloquea hasta que el contexto sea cancelado.
	rt := runner.last()
	rt.promptHook = func(ctx context.Context, _ string) error {
		<-ctx.Done()
		return ctx.Err()
	}

	start := time.Now()
	err = m.Prompt(ctx, sess.ID, "hola")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("esperaba error, obtuve nil")
	}
	if elapsed > time.Second {
		t.Fatalf("Prompt tardó demasiado: %s (esperaba ≤ 1s)", elapsed)
	}
	if !errors.Is(err, errPromptTerminated) {
		t.Fatalf("err = %v, want wraps errPromptTerminated", err)
	}
}

func TestManagerPrompt_NonTimeoutRuntimeErrorIsPropagated(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner)

	ctx := context.Background()
	sess, err := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Ensure(ctx, sess.ID); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	customErr := errors.New("error personalizado")

	rt := runner.last()
	rt.promptErr = customErr

	err = m.Prompt(ctx, sess.ID, "hola")
	if err == nil {
		t.Fatal("esperaba error, obtuve nil")
	}
	if errors.Is(err, errPromptTerminated) {
		t.Fatal("err de runtime no debe etiquetarse como timeout")
	}
}

// TestManagerPool_DefaultSizeIsOne garantiza que el cap por defecto es
// 1: abrir 2 chats debe producir 2 spawns (porque el segundo evicta
// al primero) y dejar vivo sólo 1 runtime al final. Sin este default
// "1 proceso pi por chat usado" vuelve a colarse.
func TestManagerPool_DefaultSizeIsOne(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner) // sin WithPoolSize → default 1
	if m.poolSize != 1 {
		t.Fatalf("poolSize default = %d, want 1", m.poolSize)
	}

	ctx := context.Background()
	a, _ := m.Create(ctx, CreateSessionInput{Title: "A", CWD: os.TempDir()})
	b, _ := m.Create(ctx, CreateSessionInput{Title: "B", CWD: os.TempDir()})

	if err := m.Ensure(ctx, a.ID); err != nil {
		t.Fatalf("Ensure A: %v", err)
	}
	if err := m.Ensure(ctx, b.ID); err != nil {
		t.Fatalf("Ensure B (evicta A): %v", err)
	}

	if got, want := runner.count(), 2; got != want {
		t.Fatalf("Start llamado %d veces tras evict, want %d", got, want)
	}

	// Tras la eviction, sólo el slot de B debe seguir vivo. El de A
	// fue matado por Close() durante la eviction.
	first := runner.created[0]
	second := runner.created[1]
	if !first.State().Closed {
		t.Fatal("runtime de A debería estar cerrada tras la eviction")
	}
	if second.State().Closed {
		t.Fatal("runtime de B debería estar viva")
	}
}

// TestManagerPool_SizeAllowsConcurrency: con pool=2 podemos tener 2
// chats vivos a la vez sin evict. La métrica es "no se mató nada
// entre las dos Ensure".
func TestManagerPool_SizeAllowsConcurrency(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner).WithPoolSize(2)

	ctx := context.Background()
	a, _ := m.Create(ctx, CreateSessionInput{Title: "A", CWD: os.TempDir()})
	b, _ := m.Create(ctx, CreateSessionInput{Title: "B", CWD: os.TempDir()})

	if err := m.Ensure(ctx, a.ID); err != nil {
		t.Fatalf("Ensure A: %v", err)
	}
	if err := m.Ensure(ctx, b.ID); err != nil {
		t.Fatalf("Ensure B: %v", err)
	}

	if got, want := runner.count(), 2; got != want {
		t.Fatalf("Start llamado %d veces, want %d", got, want)
	}
	for i, rt := range runner.created {
		if rt.State().Closed {
			t.Fatalf("runtime %d cerrada prematuramente con pool=2", i)
		}
	}
}

// TestManagerPool_ReusesSameSessionSlot: con pool=1, asegurar la
// misma sesión 3 veces NO debe evictar; sólo 1 spawn. Esto es el
// happy path que justifica el pool: el runtime se reutiliza para
// el mismo chat, no se mata ni se respawnea entre turnos.
func TestManagerPool_ReusesSameSessionSlot(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner) // pool=1

	ctx := context.Background()
	sess, _ := m.Create(ctx, CreateSessionInput{Title: "t", CWD: os.TempDir()})

	for i := 0; i < 3; i++ {
		if err := m.Ensure(ctx, sess.ID); err != nil {
			t.Fatalf("Ensure #%d: %v", i, err)
		}
	}
	if got, want := runner.count(), 1; got != want {
		t.Fatalf("Start llamado %d veces, want %d (mismo sessionID, pool=1)", got, want)
	}
}

// TestManagerPool_LRUEvictsOldest: con pool=1 y 3 sesiones, asegurar
// A, B, C en ese orden. Al final el slot vivo debe ser el de C; A y
// B deben haber sido cerradas (B por la entrada de C, A por la de B).
func TestManagerPool_LRUEvictsOldest(t *testing.T) {
	store := newStubStore()
	runner := &factoryRunner{}
	m := NewManager(store, runner) // pool=1

	ctx := context.Background()
	a, _ := m.Create(ctx, CreateSessionInput{Title: "A", CWD: os.TempDir()})
	b, _ := m.Create(ctx, CreateSessionInput{Title: "B", CWD: os.TempDir()})
	c, _ := m.Create(ctx, CreateSessionInput{Title: "C", CWD: os.TempDir()})

	if err := m.Ensure(ctx, a.ID); err != nil {
		t.Fatalf("Ensure A: %v", err)
	}
	if err := m.Ensure(ctx, b.ID); err != nil {
		t.Fatalf("Ensure B: %v", err)
	}
	if err := m.Ensure(ctx, c.ID); err != nil {
		t.Fatalf("Ensure C: %v", err)
	}

	if got, want := runner.count(), 3; got != want {
		t.Fatalf("Start llamado %d veces, want %d", got, want)
	}
	// A y B fueron evicted; C es el último y debe estar vivo.
	if !runner.created[0].State().Closed {
		t.Fatal("runtime de A (más vieja) debería estar cerrada")
	}
	if !runner.created[1].State().Closed {
		t.Fatal("runtime de B debería estar cerrada tras entrada de C")
	}
	if runner.created[2].State().Closed {
		t.Fatal("runtime de C (la más nueva) debería estar viva")
	}
}
