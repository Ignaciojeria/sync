package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
var previewHealthcheckTimeout = 2 * time.Second

// errPromptTerminated se devuelve cuando la runtime fue matada por timeout
// durante un send (ver pirpc.send). El caller puede decidir si reintentar.
var errPromptTerminated = fmt.Errorf("agent runtime terminated by timeout")

// ponytail: pi runtime rechaza nuevos prompts mientras está ocupado
// con un tool. El wrapper Go lo intercepta y hace steer transparente
// para que el cliente nunca vea ese error. Detectamos por substring
// porque pi devuelve strings, no sentinels.
func isAlreadyProcessing(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "already processing") ||
		strings.Contains(text, "agent is already")
}

// ErrResumeUnavailable indica que el host no pudo reconstruir contexto
// suficiente para retomar un turno existente de forma segura.
var ErrResumeUnavailable = errors.New("agent resume unavailable")
var ErrPreviewUnavailable = errors.New("agent preview unavailable")
var ErrPreviewLoopback = errors.New("agent preview cannot point to host itself")
var ErrPreviewNotApplicable = errors.New("agent preview is not applicable")
var ErrPreviewNotMergeable = errors.New("agent preview is not mergeable")
var ErrPreviewAlreadyMerged = errors.New("agent preview already merged")
var ErrPreviewMergeConflict = errors.New("agent preview merge conflict")
var ErrPreviewMergeBlocked = errors.New("agent preview merge blocked")

// RegisterPreviewInput define el upstream local que una sesión puede exponer.
type RegisterPreviewInput struct {
	Port       int    `json:"port"`
	HealthPath string `json:"healthPath"`
}

// AgentService es el contrato público que internal/agent expone hacia el host
// (cmd/api/main.go o cualquier embedder). El Manager lo satisface; cuando un
// proyecto derivado quiera mockear, sustituir o aislar el agente (ej. para
// correrlo en un sidecar o reemplazarlo por una implementación propia del
// Runner), sólo necesita satisfacer esta interfaz.
//
// Decidir mediante interfaz qué ofrecemos hacia afuera deja explícito el
// boundary: el host no toca internals de internal/agent y internal/agent no toca
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

	// Prompt, PromptRequest, Steer, Abort: acciones hacia la sesión activa.
	Prompt(ctx context.Context, id, message string) error
	PromptRequest(ctx context.Context, id string, input PromptInput) error
	Steer(ctx context.Context, id, message string) error
	Abort(ctx context.Context, id string) error
	// Regenerate re-envía el último prompt del user y borra
	// las respuestas del assistant que vinieron después. El
	// cliente recibe un envelope SSE kind="regenerate" con el
	// seq del último user prompt para borrar los items del feed.
	Regenerate(ctx context.Context, id string) error

	// Subscribe devuelve un canal de eventos + cancel para SSE.
	Subscribe(ctx context.Context, id string) (<-chan Event, func(), error)

	// RegisterPreview/ClearPreview administran el preview HTTP asociado a la sesión.
	RegisterPreview(ctx context.Context, id string, input RegisterPreviewInput) (Session, error)
	ClearPreview(ctx context.Context, id string) (Session, error)
	ApplyPreview(ctx context.Context, id string) (ApplyResult, error)
	MergePreview(ctx context.Context, id string) (MergeResult, error)

	// Delete destruye la sesión y libera sus recursos.
	Delete(ctx context.Context, id string) error

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

type PromptAction string

const (
	PromptActionPrompt PromptAction = ""
	PromptActionResume PromptAction = "resume"
)

type PromptInput struct {
	Message   string
	Action    PromptAction
	TurnID    string
	// UserEmail identifica al humano que está chateando con el
	// agente. Se usa para construir la MemoryKey del provider
	// (mapea a un peer Honcho). Si está vacío, el provider no
	// recibe contexto por prompt (comportamiento equivalente al
	// noopProvider). El HTTP handler lo popula desde el JWT.
	UserEmail string
}

const defaultTurnSteering = "Responde breve por defecto. Si el usuario pide opinión o contexto general, da un resumen corto y útil primero. Solo haz auditoría profunda si el usuario la pide explícitamente. No sigas investigando después de empezar la respuesta final salvo que expliques claramente por qué."

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
	store          SessionStore
	runner         Runner
	now            func() time.Time
	newID          func() string
	poolSize       int
	prepareSession func(context.Context, Session) (Session, error)
	destroySession func(context.Context, Session) error
	applySession   func(context.Context, Session) (ApplyResult, error)
	mergeSession   func(context.Context, Session) (MergeResult, error)
	memory         MemoryProvider
	memoryWorkspaceID string

	mu               sync.Mutex
	runtimes         map[string]*runtimeSlot
	subscribers      map[string]map[chan Event]struct{}
	assistantPreview map[string]string
	pendingInputs    map[string]PromptInput
}

// runtimeSlot une un Runtime vivo con la metadata necesaria para
// elegir quién se evicta cuando el pool se llena, y para
// coordinar la inyección de memoria por turno.
type runtimeSlot struct {
	runtime        Runtime
	lastUsedAt     time.Time
	defaultSteered bool
	// memorySteeredTurn es el TurnID del último prompt al que ya
	// se le inyectó contexto de memoria. Si el próximo PromptInput
	// trae el mismo TurnID, no re-steereamos. Se resetea cuando
	// llega un TurnID distinto.
	memorySteeredTurn string
	// lastHonchoSeq es el Seq máximo del ConversationItem que ya
	// flusheamos a Honcho vía Remember. Items con seq <= a este
	// no se reenvían. Vive en el slot porque la metadata se
	// pierde al evictar; en ese caso el próximo flush
	// potencialmente duplica, pero Honcho deduplica
	// semánticamente vía deriver.
	lastHonchoSeq     uint64
	lastHonchoItemIdx int // cantidad de items ya enviados (maneja el caso seq=0 del MaterializeUserPrompt)
	// flushInProgress evita que dos goroutines de flush corran
	// en paralelo para el mismo slot. turn_end y agent_end
	// pueden dispararse casi simultáneos; sin este flag, las dos
	// ven lastHonchoItemIdx=0 antes de que la primera setee el
	// marker y dupican todos los items a Honcho.
	flushInProgress bool
}

func NewManager(store SessionStore, runner Runner) *Manager {
	return &Manager{
		store:            store,
		runner:           runner,
		now:              time.Now,
		newID:            defaultNewID,
		poolSize:         defaultRuntimePoolSize,
		memory:           noopProvider{},
		runtimes:         map[string]*runtimeSlot{},
		subscribers:      map[string]map[chan Event]struct{}{},
		assistantPreview: map[string]string{},
		pendingInputs:    map[string]PromptInput{},
	}
}

// WithMemory reemplaza el provider de memoria. Default: noopProvider{}.
// Pensado para usar en main antes del primer prompt; después de eso
// el provider es inmutable.
func (m *Manager) WithMemory(p MemoryProvider) *Manager {
	if p == nil {
		p = noopProvider{}
	}
	m.memory = p
	return m
}

// WithMemoryWorkspace setea el Honcho WorkspaceID (u otro
// identificador de tenant) usado para construir la MemoryKey de
// cada prompt. Default: vacío, en cuyo caso el provider no se
// llama (equivale a noopProvider).
func (m *Manager) WithMemoryWorkspace(id string) *Manager {
	m.memoryWorkspaceID = strings.TrimSpace(id)
	return m
}

// hasRealMemory devuelve true cuando el provider configurado no
// es el noop default. Usado por ensureRuntime para setear
// DisableNativeHonchoTools=true en el StartSpec sólo cuando
// realmente hay un provider que enruta memoria desde el host.
func (m *Manager) hasRealMemory() bool {
	if m.memory == nil {
		return false
	}
	_, isNoop := m.memory.(noopProvider)
	return !isNoop
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

func (m *Manager) WithSessionPreparer(fn func(context.Context, Session) (Session, error)) *Manager {
	m.prepareSession = fn
	return m
}

func (m *Manager) WithSessionDestroyer(fn func(context.Context, Session) error) *Manager {
	m.destroySession = fn
	return m
}

func (m *Manager) WithSessionApplier(fn func(context.Context, Session) (ApplyResult, error)) *Manager {
	m.applySession = fn
	return m
}

func (m *Manager) WithSessionMerger(fn func(context.Context, Session) (MergeResult, error)) *Manager {
	m.mergeSession = fn
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
	if m.prepareSession != nil {
		var err error
		session, err = m.prepareSession(ctx, session)
		if err != nil {
			return Session{}, err
		}
	}
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

func (m *Manager) RegisterPreview(ctx context.Context, id string, input RegisterPreviewInput) (Session, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if input.Port < 1 || input.Port > 65535 {
		return Session{}, fmt.Errorf("invalid preview port: %d", input.Port)
	}
	healthPath := strings.TrimSpace(input.HealthPath)
	if healthPath == "" {
		healthPath = "/"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	if _, err := HealthcheckPreview(input.Port, healthPath); err != nil {
		return Session{}, fmt.Errorf("%w: %v", ErrPreviewUnavailable, err)
	}
	session = m.ensureSessionDefaults(ctx, session)
	session.PreviewPort = input.Port
	session.PreviewHealth = healthPath
	session.PreviewStatus = PreviewStatusLive
	session.PreviewURL = previewPublicURL(session.ID)
	session.UpdatedAt = m.now()
	if err := m.store.Update(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (m *Manager) ClearPreview(ctx context.Context, id string) (Session, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	session = m.ensureSessionDefaults(ctx, session)
	session.PreviewURL = ""
	session.PreviewPort = 0
	session.PreviewHealth = ""
	session.PreviewStatus = PreviewStatusNone
	session.UpdatedAt = m.now()
	if err := m.store.Update(ctx, session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (m *Manager) ApplyPreview(ctx context.Context, id string) (ApplyResult, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return ApplyResult{}, err
	}
	if m.applySession == nil {
		return ApplyResult{}, ErrPreviewNotApplicable
	}
	if strings.TrimSpace(session.WorkspacePath) == "" || strings.TrimSpace(session.SourcePath) == "" {
		return ApplyResult{}, ErrPreviewNotApplicable
	}
	return m.applySession(ctx, session)
}

func (m *Manager) MergePreview(ctx context.Context, id string) (MergeResult, error) {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return MergeResult{}, err
	}
	if m.mergeSession == nil {
		return MergeResult{}, ErrPreviewNotMergeable
	}
	if strings.TrimSpace(session.Branch) == "" || strings.TrimSpace(session.BaseBranch) == "" {
		return MergeResult{}, ErrPreviewNotMergeable
	}
	if session.MergedAt != nil || strings.TrimSpace(session.MergedCommit) != "" {
		return MergeResult{}, ErrPreviewAlreadyMerged
	}
	result, err := m.mergeSession(ctx, session)
	if err != nil {
		text := strings.ToLower(strings.TrimSpace(err.Error()))
		switch {
		case strings.Contains(text, "already merged"):
			return MergeResult{}, ErrPreviewAlreadyMerged
		case strings.Contains(text, "uncommitted changes"), strings.Contains(text, "not mergeable"):
			return MergeResult{}, fmt.Errorf("%w: %v", ErrPreviewMergeBlocked, err)
		case strings.Contains(text, "merge"):
			return MergeResult{}, fmt.Errorf("%w: %v", ErrPreviewMergeConflict, err)
		default:
			return MergeResult{}, err
		}
	}
	// ponytail: "no changes" es un éxito sin integración real. No
	// seteamos MergedAt/MergedCommit ni actualizamos branches (la
	// sesión sigue disponible para merges futuros, y un eventual
	// merge real los volverá a llenar). Devolvemos el resultado
	// tal cual para que el bar muestre "Up to date".
	if result.NoChanges {
		return result, nil
	}
	session.MergedAt = ptrTime(m.now())
	session.MergedCommit = strings.TrimSpace(result.Commit)
	if strings.TrimSpace(result.BaseBranch) != "" {
		session.BaseBranch = strings.TrimSpace(result.BaseBranch)
	}
	if strings.TrimSpace(result.PreviewBranch) != "" {
		session.Branch = strings.TrimSpace(result.PreviewBranch)
	}
	session.UpdatedAt = m.now()
	if err := m.store.Update(ctx, session); err != nil {
		return MergeResult{}, err
	}
	return result, nil
}

// Prompt envía un mensaje al runtime asociado a la sesión. Aplica un timeout
// acotado independiente del request context para que un pi atascado no bloquee
// al handler HTTP. Si el runtime es terminado por el timeout (pirpc.detecta
// un bloqueo en stdin), el Manager devuelve errPromptTerminated; el siguiente
// Prompt disparará un ensureRuntime que spawnará un proceso nuevo.
func (m *Manager) Prompt(ctx context.Context, id, message string) error {
	return m.PromptRequest(ctx, id, PromptInput{Message: message})
}

// Regenerate re-envía el último prompt del user de la sesión,
// borrando las respuestas del assistant que vinieron después.
// El cliente V2 recibe un envelope SSE kind="regenerate" con
// el seq del último user prompt; borra los items del feed con
// seq mayor a ese, y los nuevos fragments del assistant
// llegan con seqs nuevos que se renderizan normalmente.
//
// Limitaciones (M-C.2 simple):
// - Solo funciona para el ÚLTIMO prompt del user. No se
//   puede regenerar respuestas intermedias (más invasivo,
//   requiere redesign del state machine).
// - Los side effects de tools ejecutadas en el turno viejo
//   (bash, write, etc) NO se rollbackean. El agente puede
//   re-ejecutar tools side-effectful — para casos como
//   "git commit" eso es problemático. Queda como
//   responsabilidad del agente (y del user) validar el
//   contexto antes de regenerar.
func (m *Manager) Regenerate(ctx context.Context, id string) error {
	items, err := readConversationTranscript(id)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return errors.New("regenerate: transcript vacío")
	}
	// ponytail: encontramos el último user prompt y borramos
	// todo lo que viene después. Buscamos desde el final
	// hacia atrás porque si el último item es un assistant,
	// ese es el que queremos regenerar (su prompt es el
	// user inmediatamente anterior).
	var lastUserSeq uint64
	var lastUserText string
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind == "user" && items[i].Text != "" {
			lastUserSeq = items[i].Seq
			lastUserText = items[i].Text
			break
		}
	}
	if lastUserText == "" {
		return errors.New("regenerate: no hay user prompt en el transcript")
	}
	// Borramos todos los items con seq > lastUserSeq. El
	// último user prompt queda; los nuevos fragments van a
	// appendear con seqs nuevos desde lastUserSeq+1.
	if err := truncateTranscriptAfter(id, lastUserSeq); err != nil {
		return err
	}
	// Re-enviamos el prompt. PromptRequest maneja el caso
	// de runtime running (queuePendingInput) y el caso de
	// runtime caído (ensureRuntime spawnea uno nuevo).
	return m.PromptRequest(ctx, id, PromptInput{Message: lastUserText})
}

func (m *Manager) PromptRequest(ctx context.Context, id string, input PromptInput) error {
	message, err := m.resolvePromptMessage(ctx, id, input)
	if err != nil {
		return err
	}
	input.Message = message

	session, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if session.Status == SessionStatusRunning {
		m.queuePendingInput(id, input)
		return nil
	}

	runtime, session, err := m.ensureRuntime(ctx, id)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, promptCallTimeout)
	defer cancel()

	// Default steering desactivado temporalmente (Fase 1 fix
	// LLM coherence): el steer de 280 chars en español descarrila
	// al modelo "minimax/m3" en sesiones >500 turnos (probado
	// vía cmd/spike-pigo en branch feature/pi-go-migration). El
	// bug se manifiesta como respuestas incoherentes o hangs.
	// La función ensureDefaultSteering queda definida por si
	// queremos re-habilitar via flag o mover a --append-system-prompt.
	// if err := m.ensureDefaultSteering(callCtx, id, runtime); err != nil {
	// 	m.setSessionStatus(ctx, session, SessionStatusError)
	// 	return err
	// }

	// Inyección de memoria Honcho (opt-in, best-effort). Un error
	// del provider NO bloquea el prompt — loggeamos y seguimos.
	// El slot marker evita re-steerear con el mismo contexto si
	// pi invoca Prompt varias veces para el mismo TurnID (típico
	// cuando hay queuePendingInput).
	if err := m.injectMemoryRecall(callCtx, id, runtime, input); err != nil {
		slog.Warn("agent: memory recall failed; continuing without",
			"session_id", id,
			"err", err,
		)
	}

	if err := runtime.Prompt(callCtx, message); err != nil {
		if isAlreadyProcessing(err) {
			// ponytail: pi está ejecutando otro tool. Hacemos steer
			// transparente y NO marcamos la sesión como error — el
			// runtime sigue vivo. Si steer también falla, retornamos
			// ese error y dejamos que el caller decida. Esto reemplaza
			// la lógica de "already processing" que el cliente JS
			// tenía que orquestar manualmente (ver isAlreadyProcessingError,
			// recoverBusyTurn, holdBusyUntilTurnEnd en page.templ).
			return runtime.Steer(callCtx, message)
		}
		if isCallTimeout(callCtx, err) {
			m.setSessionStatus(ctx, session, SessionStatusError)
			slog.Warn("agent prompt killed by timeout",
				"session_id", id,
				"action", strings.TrimSpace(string(input.Action)),
				"err", err.Error(),
			)
			return fmt.Errorf("%w: %v", errPromptTerminated, err)
		}
		m.setSessionStatus(ctx, session, SessionStatusError)
		return err
	}
	m.setSessionPreview(ctx, id, "user", message, 0)
	// ponytail: NO llamamos MaterializeUserPrompt acá — el sendPrompt
	// handler (POST /prompt) ya lo escribió al inicio. Si lo
	// llamáramos también, sería no-op por dedup (readLastTranscriptLine),
	// pero es ruido y confunde el path. El user prompt se persiste
	// en el sendPrompt path SIEMPRE, antes de runtime.Prompt, así
	// que sobrevive a timeouts y errores.
	m.setSessionStatus(ctx, session, SessionStatusRunning)
	return nil
}

func (m *Manager) resolvePromptMessage(ctx context.Context, id string, input PromptInput) (string, error) {
	message := strings.TrimSpace(input.Message)
	if message == "" {
		return "", errors.New("message is required")
	}
	if input.Action != PromptActionResume {
		return message, nil
	}

	history, err := LoadConversationHistoryCtx(ctx, id, 0, 30)
	if err != nil {
		return "", err
	}
	lastAssistant := lastAssistantText(history.Items)
	if strings.TrimSpace(lastAssistant) == "" {
		return "", fmt.Errorf("%w: no assistant turn to resume", ErrResumeUnavailable)
	}
	return buildResumeContinuationPrompt(lastAssistant), nil
}

func lastAssistantText(items []ConversationItem) string {
	for i := len(items) - 1; i >= 0; i-- {
		if strings.TrimSpace(items[i].Kind) != "assistant" {
			continue
		}
		text := strings.TrimSpace(items[i].Text)
		if text != "" {
			return text
		}
	}
	return ""
}

func buildResumeContinuationPrompt(lastAssistant string) string {
	return fmt.Sprintf(
		"Tu respuesta anterior fue interrumpida.\n\n"+
			"Continúa exactamente desde este punto, sin repetir ni reformular lo ya dicho:\n"+
			"%q\n\n"+
			"No empieces de nuevo. No agregues introducción. Sigue el mismo párrafo o lista.",
		resumeTail(lastAssistant, 400),
	)
}

func resumeTail(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return "…" + strings.TrimSpace(text[len(text)-max:])
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

func (m *Manager) Delete(ctx context.Context, id string) error {
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return err
	}

	m.mu.Lock()
	slot, ok := m.runtimes[id]
	if ok {
		delete(m.runtimes, id)
	}
	delete(m.subscribers, id)
	delete(m.assistantPreview, id)
	delete(m.pendingInputs, id)
	m.mu.Unlock()

	if ok {
		_ = slot.runtime.Close()
	}
	if m.destroySession != nil {
		if err := m.destroySession(ctx, session); err != nil {
			return err
		}
	}
	if err := m.store.Delete(ctx, id); err != nil {
		return err
	}
	return nil
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
		// AgentID quedó normalizado en Create o via backfill
		// en ensureSessionDefaults. Si por algún motivo llega
		// vacío acá, caemos al default del registry para que
		// el runner nunca siembre desde una ruta inesperada.
		AgentID:                session.AgentID,
		// Si el host ya enruta memoria vía MemoryProvider
		// real (no noop), pedimos al runner que no cargue
		// la extensión Honcho nativa de .pi/extensions/ para
		// evitar doble consumo. Forward-compat: hoy esa
		// extensión no existe en este repo, pero el filtro
		// queda en pirpc.Process.
		DisableNativeHonchoTools: m.hasRealMemory(),
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

func (m *Manager) queuePendingInput(id string, input PromptInput) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pendingInputs[id] = input
}

func (m *Manager) popPendingInput(id string) (PromptInput, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	input, ok := m.pendingInputs[id]
	if ok {
		delete(m.pendingInputs, id)
	}
	return input, ok
}

func (m *Manager) ensureDefaultSteering(ctx context.Context, id string, runtime Runtime) error {
	m.mu.Lock()
	slot := m.runtimes[id]
	if slot == nil || slot.defaultSteered {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	if err := runtime.Steer(ctx, defaultTurnSteering); err != nil {
		slog.Warn("agent: default steering failed",
			"session_id", id,
			"err", err,
		)
		return err
	}

	m.mu.Lock()
	if slot := m.runtimes[id]; slot != nil && slot.runtime == runtime {
		slot.defaultSteered = true
	}
	m.mu.Unlock()
	return nil
}

// injectMemoryRecall es la pieza central de Fase C. Construye
// la MemoryKey, garantiza que los peers existan en Honcho (best
// effort), trae contexto relevante al mensaje y lo inyecta como
// steer si no está vacío. Re-steerear con el mismo TurnID es
// no-op (slot marker).
//
// Garantías:
//   - NO bloquea el prompt si el provider falla (sigue sin memoria).
//   - NO se llama si UserEmail o memoryWorkspaceID están vacíos.
//   - NO se llama si la MemoryKey quedó igual que el último
//     TurnID ya steereado en este slot.
//
// Devuelve error siempre que el provider falló; el caller decide
// si loggear o propagar.
func (m *Manager) injectMemoryRecall(ctx context.Context, id string, runtime Runtime, input PromptInput) error {
	if m.memory == nil {
		return nil
	}
	if m.memoryWorkspaceID == "" || strings.TrimSpace(input.UserEmail) == "" {
		return nil
	}
	key := MemoryKey{
		WorkspaceID: m.memoryWorkspaceID,
		SessionID:   id,
		UserEmail:   input.UserEmail,
	}

	// Slot marker: si ya steereamos este TurnID, no repetir.
	// Evita inyecciones duplicadas cuando PromptRequest se llama
	// varias veces para el mismo turn (Regenerate, queuePending,
	// etc.).
	m.mu.Lock()
	slot := m.runtimes[id]
	if slot != nil && slot.memorySteeredTurn == input.TurnID {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	// EnsurePeers es idempotente y barato. Si falla, el resto del
	// flujo (Recall) va a fallar también, así que no hace
	// diferencia cortocircuitar acá.
	if err := m.memory.EnsurePeers(ctx, key); err != nil {
		return fmt.Errorf("ensure peers: %w", err)
	}

	mem, err := m.memory.Recall(ctx, key, input.Message)
	if err != nil {
		return fmt.Errorf("recall: %w", err)
	}
	if strings.TrimSpace(mem.Text) == "" {
		return nil
	}

	steer := "Memoria relevante (de sesiones previas con este usuario):\n" + mem.Text
	if err := runtime.Steer(ctx, steer); err != nil {
		return fmt.Errorf("steer memory: %w", err)
	}

	// Marcamos el slot. Si el runtime fue reemplazado entre la
	// lectura y la escritura, el steer quedó en el runtime
	// anterior (ya cerrado); no es recuperable, sólo loggeamos.
	m.mu.Lock()
	if s := m.runtimes[id]; s != nil && s.runtime == runtime {
		s.memorySteeredTurn = input.TurnID
	} else {
		slog.Warn("agent: runtime was replaced after memory steer",
			"session_id", id,
		)
	}
	m.mu.Unlock()
	return nil
}

// flushMemoryRemember persiste los mensajes user/assistant
// del transcript desde el último flush. Lo llama el handler de
// turn_end/agent_end en background; cualquier error es
// warning-only porque perder un flush es aceptable para v1.
func (m *Manager) flushMemoryRemember(ctx context.Context, id string) {
	if m.memory == nil {
		return
	}
	if m.memoryWorkspaceID == "" {
		return
	}

	// Single-flight por slot: si ya hay un flush corriendo para
	// esta session, salimos. Sin esto, turn_end y agent_end
	// race-condition y dupican mensajes en Honcho.
	m.mu.Lock()
	slot := m.runtimes[id]
	if slot == nil {
		// Slot evictado entre el evento y el flush. Reanclar
		// uno nuevo para tracking.
		slot = &runtimeSlot{}
		m.runtimes[id] = slot
	}
	if slot.flushInProgress {
		m.mu.Unlock()
		return
	}
	slot.flushInProgress = true
	startIdx := slot.lastHonchoItemIdx
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		if s := m.runtimes[id]; s != nil {
			s.flushInProgress = false
		}
		m.mu.Unlock()
	}()

	session, err := m.store.Get(ctx, id)
	if err != nil {
		slog.Warn("agent: memory flush: store get failed",
			"session_id", id, "err", err)
		return
	}
	// OwnerEmail de la session, no de PromptInput: el flush
	// ocurre al final del turno, después de que el prompt ya
	// pasó. El dueño de la session es el humano del chat.
	userEmail := strings.TrimSpace(session.OwnerEmail)
	if userEmail == "" {
		// Sin owner no podemos construir la key; abortamos
		// silenciosamente. (caso: session creada por dev sin
		// pasar email; el provider es noop de todas formas.)
		return
	}

	history, err := LoadConversationHistoryCtx(ctx, id, 0, 0) // 0 = todo
	if err != nil {
		slog.Warn("agent: memory flush: load history failed",
			"session_id", id, "err", err)
		return
	}

	key := MemoryKey{
		WorkspaceID: m.memoryWorkspaceID,
		SessionID:   id,
		UserEmail:   userEmail,
	}

	// Filtramos por índice (no por Seq) porque
	// MaterializeUserPrompt graba items con Seq=0, lo que haría
	// que un filter basado en Seq no-envíe nada en el primer
	// flush. El transcript es append-only y LoadConversation
	// devuelve los items en orden estable, así que el índice
	// es monotónico.
	if startIdx > len(history.Items) {
		startIdx = 0 // el slot se evictó; mandamos todo de nuevo. Honcho deduplica semánticamente.
	}
	var (
		msgs []MemoryMessage
	)
	for i := startIdx; i < len(history.Items); i++ {
		item := history.Items[i]
		if item.Kind != "user" && item.Kind != "assistant" {
			continue
		}
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		msgs = append(msgs, MemoryMessage{
			Role:      item.Kind,
			Text:      text,
			CreatedAt: session.UpdatedAt, // aproximación; Honcho no usa timestamp estricto
		})
	}
	if len(msgs) == 0 {
		return
	}

	if err := m.memory.Remember(ctx, key, msgs); err != nil {
		slog.Warn("agent: memory flush: remember failed",
			"session_id", id,
			"count", len(msgs),
			"err", err,
		)
		return
	}

	m.mu.Lock()
	if s := m.runtimes[id]; s != nil {
		s.lastHonchoItemIdx = len(history.Items)
	}
	m.mu.Unlock()
}

func (m *Manager) setSessionPreview(ctx context.Context, id, kind, text string, seq uint64) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return
	}
	session = m.ensureSessionDefaults(ctx, session)
	session.LastPreview = previewText(cleanMarkdownForPreview(text), 600)
	session.LastPreviewKind = strings.TrimSpace(kind)
	if seq > 0 {
		session.LastSeq = seq
	}
	session.UpdatedAt = m.now()
	_ = m.store.Update(ctx, session)
}

func (m *Manager) SetLastSeq(ctx context.Context, id string, seq uint64) {
	if seq == 0 {
		return
	}
	session, err := m.store.Get(ctx, id)
	if err != nil {
		return
	}
	session = m.ensureSessionDefaults(ctx, session)
	session.LastSeq = seq
	session.UpdatedAt = m.now()
	_ = m.store.Update(ctx, session)
}

func (m *Manager) capturePreviewEvent(id string, event Event) {
	ctx := context.Background()
	switch event.Type {
	case "message_start":
		m.mu.Lock()
		m.assistantPreview[id] = ""
		m.mu.Unlock()
	case "message_update":
		var payload struct {
			AssistantMessageEvent struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			} `json:"assistantMessageEvent"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil || payload.AssistantMessageEvent.Type != "text_delta" {
			return
		}
		m.mu.Lock()
		m.assistantPreview[id] = mergeAssistantDelta(m.assistantPreview[id], payload.AssistantMessageEvent.Delta)
		text := visibleAssistantText(m.assistantPreview[id])
		m.mu.Unlock()
		if strings.TrimSpace(text) != "" {
			m.setSessionPreview(ctx, id, "assistant", text, 0)
		}
	case "message_end":
		m.mu.Lock()
		delete(m.assistantPreview, id)
		m.mu.Unlock()
	case "turn_end", "agent_end":
		m.markSessionIdleAndFlushPending(ctx, id)
		// Flush de memoria Honcho en background. Si el adapter
		// es noop, el método es no-op silencioso. No bloquea el
		// event loop de capturePreviewEvent.
		go m.flushMemoryRemember(context.Background(), id)
	case "tool_execution_start":
		var payload struct {
			ToolName string `json:"toolName"`
			Type     string `json:"type"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		name := strings.TrimSpace(payload.ToolName)
		if name == "" {
			name = strings.TrimSpace(payload.Type)
		}
		if name != "" {
			m.setSessionPreview(ctx, id, "tool", "Tool: "+name, 0)
		}
	case "runtime_error", "stderr", "runtime_exit":
		if event.Type == "runtime_exit" {
			m.markSessionIdleAndFlushPending(ctx, id)
		}
		var payload struct {
			Message json.RawMessage `json:"message"`
			Reason  string          `json:"reason"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		text := extractMessageText(payload.Message)
		if text == "" {
			text = strings.TrimSpace(payload.Reason)
		}
		if text != "" {
			m.setSessionPreview(ctx, id, "error", text, 0)
		}
	}
}

func (m *Manager) ensureSessionDefaults(ctx context.Context, session Session) Session {
	changed := false
	if strings.TrimSpace(session.PiSessionFile) == "" {
		session.PiSessionFile = DefaultPiSessionFile(session.ID)
		changed = strings.TrimSpace(session.PiSessionFile) != ""
	}
	if session.PreviewPort > 0 && strings.TrimSpace(session.PreviewURL) == "" {
		session.PreviewURL = previewPublicURL(session.ID)
		changed = true
	}
	// Backfill AgentID para sesiones creadas antes de que el
	// multi-agente existiera (no tenían el campo). Sin esto,
	// pirpc.resolveCWD recibiría AgentID="" y caería al
	// default, pero queremos que el Session persistido refleje
	// la realidad para que el cliente y el runner sean
	// consistentes.
	if strings.TrimSpace(session.AgentID) == "" {
		session.AgentID = DefaultAgentID()
		changed = true
	}
	if changed {
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
	m.capturePreviewEvent(id, event)

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

func previewPublicURL(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "/agent/sessions/" + id + "/preview/"
}

func HealthcheckPreview(port int, healthPath string) (int, error) {
	healthPath = strings.TrimSpace(healthPath)
	if healthPath == "" {
		healthPath = "/"
	}
	if !strings.HasPrefix(healthPath, "/") {
		healthPath = "/" + healthPath
	}
	reqCtx, cancel := context.WithTimeout(context.Background(), previewHealthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", port, healthPath), nil)
	if err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusInternalServerError {
		return resp.StatusCode, fmt.Errorf("healthcheck returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (m *Manager) markSessionIdleAndFlushPending(ctx context.Context, id string) {
	session, err := m.store.Get(ctx, id)
	if err == nil {
		session = m.ensureSessionDefaults(ctx, session)
		m.setSessionStatus(ctx, session, SessionStatusIdle)
	}
	input, ok := m.popPendingInput(id)
	if !ok {
		return
	}
	go func() {
		if err := m.PromptRequest(context.Background(), id, input); err != nil {
			slog.Warn("agent: failed to flush queued prompt",
				"session_id", id,
				"action", strings.TrimSpace(string(input.Action)),
				"err", err,
			)
		}
	}()
}
