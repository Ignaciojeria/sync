package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// promptCallTimeout es la cota superior que Manager aplica a las llamadas
// hacia la runtime (prompt/steer/abort). El handler HTTP ya tiene su propio
// timeout (read/write), pero queremos cortar el ciclo antes para que la UI no
// quede colgada. Se deja como var para que los tests puedan ajustarlo sin
// esperar 30 s.
var promptCallTimeout = 30 * time.Second

// errPromptTerminated se devuelve cuando la runtime fue matada por timeout
// durante un send (ver pirpc.send). El caller puede decidir si reintentar.
var errPromptTerminated = fmt.Errorf("agent runtime terminated by timeout")

// AgentService es el contrato público que pkg/agent expone hacia el host
// (cmd/api/main.go o cualquier embedder). El Manager lo satisface; cuando un
// proyecto derivado quiera mockear, sustituir o aislar el agente (ej. para
// correrlo en un sidecar o reemplazarlo por una implementación propia del
// Runner), sólo necesita satisfacer esta interfaz.
//
// Decidir mediante interfaz qué ofrecemos hacia afuera deja explícito el
// boundary: el host no toca internals de pkg/agent y pkg/agent no toca
// internals del host. Es la base para que el agente sea removible vía
// opt-out (build tag, env var AGENT_ENABLED=false) sin que el resto
// del boilerplate sufra cambios.
type AgentService interface {
	// List, Create, Get: CRUD de metadata de sesiones.
	List(ctx context.Context) ([]Session, error)
	Create(ctx context.Context, input CreateSessionInput) (Session, error)
	Get(ctx context.Context, id string) (Session, error)

	// Ensure pre-calienta la runtime para una sesión sin esperar a un
	// prompt. Devuelve ErrSessionNotFound si la sesión no existe.
	Ensure(ctx context.Context, id string) error

	// Prompt, Steer, Abort: acciones hacia la sesión activa.
	Prompt(ctx context.Context, id, message string) error
	Steer(ctx context.Context, id, message string) error
	Abort(ctx context.Context, id string) error

	// Subscribe devuelve un canal de eventos + cancel para SSE.
	Subscribe(ctx context.Context, id string) (<-chan Event, func(), error)

	// Close cierra todos los runtimes vivos. Llamado en shutdown.
	Close() error
}

// Compile-time check: *Manager implementa AgentService. Si rompe, el
// método que falta aparece explícito en el error de compilación.
var _ AgentService = (*Manager)(nil)

// defaultRuntimePoolSize es el cap por defecto de procesos pi vivos
// que el Manager retiene. Antes era "1 por sesión usada", lo que con
// N chats abiertos = N procesos. Ahora es 1 = el runtime sirve a
// todas las sesiones vía switch de --session; cuando llega un
// segundo chat, el LRU se evicta y se respawnea. Subir este número
// desde el caller (cmd/agent-worker) si querés paralelismo real
// entre chats.
const defaultRuntimePoolSize = 1

// Manager coordina metadata persistida y runtimes vivos del agente.
//
// Las runtimes forman un pool de tamaño acotado (default 1). Cada
// slot del pool está bound a lo sumo a una sesión; cuando un slot
// libre recibe un bind, el runner.Start lo spawnea con
// --session=<archivo>. Si el pool está lleno y entra una sesión
// nueva, se evicta la sesión menos usada recientemente (mata el
// proceso pi y la reemplaza por una fresca para la nueva sesión).
// Esto rompe 1-chat-1-proceso y baja el techo de RAM a
// `poolSize × RSS de pi`.
type Manager struct {
	store    SessionStore
	runner   Runner
	now      func() time.Time
	newID    func() string
	poolSize int

	mu          sync.Mutex
	runtimes    map[string]*runtimeSlot
	subscribers map[string]map[chan Event]struct{}
}

// runtimeSlot une un Runtime vivo con la metadata necesaria para
// elegir quién se evicta cuando el pool se llena.
type runtimeSlot struct {
	runtime    Runtime
	lastUsedAt time.Time
}

func NewManager(store SessionStore, runner Runner) *Manager {
	return &Manager{
		store:       store,
		runner:      runner,
		now:         time.Now,
		newID:       defaultNewID,
		poolSize:    defaultRuntimePoolSize,
		runtimes:    map[string]*runtimeSlot{},
		subscribers: map[string]map[chan Event]struct{}{},
	}
}

func defaultNewID() string {
	// ponytail: counter atómico para evitar colisiones cuando
	// time.Now().UnixNano() devuelve el mismo valor en llamadas
	// consecutivas (común en Windows donde la resolución del reloj
	// es del orden de ~100 ns). El prefijo sigue siendo agent-
	// para no romper asumers downstream que parsean ese formato.
	idCounter.Add(1)
	return fmt.Sprintf("agent-%d-%d", time.Now().UnixNano(), idCounter.Load())
}

var idCounter atomic.Int64

// WithPoolSize ajusta el cap del pool de runtimes. Pensado para
// usar en main antes del primer uso; después de eso el cap es
// inmutable (el Manager no lo relee).
func (m *Manager) WithPoolSize(n int) *Manager {
	if n < 1 {
		n = 1
	}
	m.poolSize = n
	return m
}

func (m *Manager) List(ctx context.Context) ([]Session, error) {
	sessions, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		sessions[i] = m.ensureSessionDefaults(ctx, sessions[i])
		sessions[i] = m.applyRuntimeState(sessions[i])
	}
	return sessions, nil
}

func (m *Manager) Create(ctx context.Context, input CreateSessionInput) (Session, error) {
	session := NewSession(m.newID(), input, m.now())
	if err := m.store.Create(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Session, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	session = m.ensureSessionDefaults(ctx, session)
	return m.applyRuntimeState(session), nil
}

// Prompt envía un mensaje al runtime asociado a la sesión. Aplica un timeout
// acotado independiente del request context para que un pi atascado no bloquee
// al handler HTTP. Si el runtime es terminado por el timeout (pirpc.detecta
// un bloqueo en stdin), el Manager devuelve errPromptTerminated; el siguiente
// Prompt disparará un ensureRuntime que spawnará un proceso nuevo.
func (m *Manager) Prompt(ctx context.Context, id, message string) error {
	runtime, session, err := m.ensureRuntime(ctx, id)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, promptCallTimeout)
	defer cancel()

	if err := runtime.Prompt(callCtx, message); err != nil {
		if isCallTimeout(callCtx, err) {
			m.setSessionStatus(ctx, session, SessionStatusError)
			slog.Warn("agent prompt killed by timeout",
				"session_id", id,
				"err", err.Error(),
			)
			return fmt.Errorf("%w: %v", errPromptTerminated, err)
		}
		m.setSessionStatus(ctx, session, SessionStatusError)
		return err
	}
	m.setSessionStatus(ctx, session, SessionStatusRunning)
	return nil
}

func (m *Manager) Steer(ctx context.Context, id, message string) error {
	runtime, session, err := m.ensureRuntime(ctx, id)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, promptCallTimeout)
	defer cancel()

	if err := runtime.Steer(callCtx, message); err != nil {
		if isCallTimeout(callCtx, err) {
			m.setSessionStatus(ctx, session, SessionStatusError)
			return fmt.Errorf("%w: %v", errPromptTerminated, err)
		}
		m.setSessionStatus(ctx, session, SessionStatusError)
		return err
	}
	m.setSessionStatus(ctx, session, SessionStatusRunning)
	return nil
}

func (m *Manager) Abort(ctx context.Context, id string) error {
	runtime, session, err := m.ensureRuntime(ctx, id)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, promptCallTimeout)
	defer cancel()

	if err := runtime.Abort(callCtx); err != nil {
		return err
	}
	m.setSessionStatus(ctx, session, SessionStatusIdle)
	return nil
}

// Ensure pre-calienta la runtime para una sesión sin esperar a un prompt.
// Sirve para que el coste de spawn del proceso pi se pague en la carga de la
// página y el primer mensaje del usuario no sufra ese retardo. Si la sesión
// no existe devuelve ErrSessionNotFound; cualquier fallo de spawn se propaga.
func (m *Manager) Ensure(ctx context.Context, id string) error {
	_, _, err := m.ensureRuntime(ctx, id)
	return err
}

func isCallTimeout(callCtx context.Context, err error) bool {
	if errors.Is(err, errPromptTerminated) {
		return true
	}
	if callCtx.Err() != nil {
		return true
	}
	return false
}

func (m *Manager) Subscribe(ctx context.Context, id string) (<-chan Event, func(), error) {
	if _, err := m.store.Get(ctx, id); err != nil {
		return nil, nil, err
	}
	ch := make(chan Event, 256)
	m.mu.Lock()
	if m.subscribers[id] == nil {
		m.subscribers[id] = map[chan Event]struct{}{}
	}
	m.subscribers[id][ch] = struct{}{}
	m.mu.Unlock()
	cancel := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if subs := m.subscribers[id]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(m.subscribers, id)
			}
		}
	}
	return ch, cancel, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	runtimes := make([]Runtime, 0, len(m.runtimes))
	for _, slot := range m.runtimes {
		runtimes = append(runtimes, slot.runtime)
	}
	m.runtimes = map[string]*runtimeSlot{}
	m.mu.Unlock()

	var firstErr error
	for _, runtime := range runtimes {
		if err := runtime.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *Manager) ensureRuntime(ctx context.Context, id string) (Runtime, Session, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return nil, Session{}, err
	}
	session = m.ensureSessionDefaults(ctx, session)

	// Fast path: la sesión ya tiene slot vivo.
	m.mu.Lock()
	slot, ok := m.runtimes[id]
	if ok && !slot.runtime.State().Closed {
		slot.lastUsedAt = m.now()
		rt := slot.runtime
		m.mu.Unlock()
		return rt, session, nil
	}
	if ok {
		// Slot marcado closed: lo limpiamos sin contar contra el cap.
		delete(m.runtimes, id)
	}
	m.mu.Unlock()

	// Si el pool está saturado, evictamos al LRU fuera del lock para
	// no bloquear a otros callers durante el kill (puede tardar si
	// la víctima está en un prompt atascado).
	if err := m.maybeEvictForNewSlot(ctx, id); err != nil {
		return nil, Session{}, err
	}

	runtime, err := m.runner.Start(context.Background(), StartSpec{
		SessionID:   session.ID,
		CWD:         session.CWD,
		Model:       session.Model,
		Title:       session.Title,
		SessionFile: session.PiSessionFile,
	})
	if err != nil {
		m.setSessionStatus(ctx, session, SessionStatusError)
		return nil, Session{}, err
	}

	m.mu.Lock()
	// Re-chequear: maybeEvict pudo haber sido no-op si el cap dejó
	// espacio, pero entre el unlock y este lock otro caller pudo
	// haber saturado el pool otra vez. En ese caso, lo más simple
	// es cerrar el runtime recién spawneado y reportar.
	if len(m.runtimes) >= m.poolSize {
		m.mu.Unlock()
		_ = runtime.Close()
		return nil, Session{}, errors.New("agent: pool saturated by concurrent caller")
	}
	m.runtimes[id] = &runtimeSlot{runtime: runtime, lastUsedAt: m.now()}
	m.mu.Unlock()
	m.attachRuntimeEvents(id, runtime)
	return runtime, session, nil
}

// maybeEvictForNewSlot saca del pool a la sesión menos usada
// recientemente para hacer lugar al bind de newSessionID. Si el
// pool ya tiene espacio, es no-op. El kill de la runtime víctima
// ocurre fuera del lock de runtimes (puede demorar).
//
// ponytail: bound a poolSize evictions para que bajo contención
// extrema (otro caller gana el slot que dejamos libre y vuelve a
// saturar) no nos quedemos en loop. Si después de poolSize
// evictions sigue saturado, devolvemos error y el caller recibe
// 500 — el handler lo reintentará naturalmente.
func (m *Manager) maybeEvictForNewSlot(_ context.Context, newSessionID string) error {
	evicted := 0
	for {
		m.mu.Lock()
		if len(m.runtimes) < m.poolSize {
			m.mu.Unlock()
			return nil
		}
		if evicted >= m.poolSize {
			m.mu.Unlock()
			return errors.New("agent: pool saturated after evictions")
		}
		victimID, victimSlot := m.pickLRU()
		if victimSlot == nil || victimID == newSessionID {
			m.mu.Unlock()
			return errors.New("agent: pool saturated")
		}
		delete(m.runtimes, victimID)
		m.mu.Unlock()

		slog.Info("agent: evicting LRU runtime to make room in pool",
			"victim_session", victimID,
			"new_session", newSessionID,
			"pool_size", m.poolSize,
		)
		// ponytail: el kill puede fallar; ya sacamos al slot del
		// mapa, así que un error de Close no rompe el bind nuevo.
		// Logueamos y seguimos.
		if err := victimSlot.runtime.Close(); err != nil {
			slog.Warn("agent: LRU close error", "session_id", victimID, "err", err)
		}
		evicted++
	}
}

// pickLRU devuelve el sessionID y slot de la sesión menos usada
// recientemente. Asume que el caller tiene el lock tomado y que el
// mapa no está vacío. Retorna ("", nil) si no hay candidatos
// (pool vacío), aunque en ese caso el caller no debería haber
// llamado.
func (m *Manager) pickLRU() (string, *runtimeSlot) {
	var lruID string
	var lruSlot *runtimeSlot
	for id, slot := range m.runtimes {
		if lruSlot == nil || slot.lastUsedAt.Before(lruSlot.lastUsedAt) {
			lruID = id
			lruSlot = slot
		}
	}
	return lruID, lruSlot
}

func (m *Manager) applyRuntimeState(session Session) Session {
	m.mu.Lock()
	slot, ok := m.runtimes[session.ID]
	m.mu.Unlock()
	if !ok {
		return session
	}
	state := slot.runtime.State()
	session.Status = SessionStatus(state.Status)
	if state.Model != "" {
		session.Model = state.Model
	}
	session.UpdatedAt = m.now()
	return session
}

func (m *Manager) setSessionStatus(ctx context.Context, session Session, status SessionStatus) {
	session.Status = status
	session.UpdatedAt = m.now()
	_ = m.store.Update(ctx, session)
}

func (m *Manager) ensureSessionDefaults(ctx context.Context, session Session) Session {
	if strings.TrimSpace(session.PiSessionFile) != "" {
		return session
	}
	session.PiSessionFile = DefaultPiSessionFile(session.ID)
	if strings.TrimSpace(session.PiSessionFile) != "" {
		_ = m.store.Update(ctx, session)
	}
	return session
}

func (m *Manager) attachRuntimeEvents(id string, runtime Runtime) {
	ch, cancel := runtime.Subscribe()
	go func() {
		defer cancel()
		for event := range ch {
			m.broadcast(id, event)
		}
	}()
}

func (m *Manager) broadcast(id string, event Event) {
	m.mu.Lock()
	subs := make([]chan Event, 0, len(m.subscribers[id]))
	for ch := range m.subscribers[id] {
		subs = append(subs, ch)
	}
	m.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			slog.Warn("agent: dropping event for slow subscriber",
				"session_id", id,
				"event_type", event.Type,
			)
		}
	}
}
