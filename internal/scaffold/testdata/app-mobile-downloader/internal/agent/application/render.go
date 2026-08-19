package application

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
)

// FragmentEvent es lo que el cliente del chat recibe por SSE.
// Reemplaza el envelope crudo del provider (pi) por cuatro tipos
// que el browser entiende sin parsear el protocolo:
//
//   - {"kind":"fragment","id":N,"html":"…"} → nuevo item
//     materializado, listo para insertAdjacentHTML.
//   - {"kind":"phase","phase":"thinking|tooling|compacting|
//     retrying|error"} → señal de fase para el badge del
//     header. El cliente mapea a un único estado "Trabajando"
//     para el primario y un detalle opcional para el secundario.
//   - {"kind":"turn-end"} → apagar dots de "generando…".
//   - {"kind":"queue","queueSteering":N,"queueFollowUp":M}
//     → metadata aditiva sobre la cola de pi. No toca el
//     phase primario; sólo refresca el badge secundario.
//
// Antes el cliente manejaba un switch sobre envelope.type con casos
// para message_start/message_update/message_end/tool_execution_start/
// turn_end/agent_end/runtime_error/runtime_exit/stderr. Cada caso
// reconstruía DOM, mutaba buffers internos, decidía qué pintar. Este
// archivo mueve esa lógica al servidor; el cliente solo recibe
// fragmentos listos para pintar, sin saber qué provider los produjo.
//
// ponytail: no inventamos una interface FragmentRenderer ni un
// registry de providers. El renderer concreto vive en agent/ui y se
// inyecta via SetFragmentRenderer al iniciar el paquete. Hoy hay un
// solo renderer; si mañana se suma otro modelo, escribimos otro
// switch en el mismo lugar.

type FragmentEvent struct {
	Kind  string `json:"kind"`
	ID    uint64 `json:"id,omitempty"`
	Phase string `json:"phase,omitempty"`
	HTML  string `json:"html,omitempty"`
	// ItemKind expone al cliente el tipo del item del transcript
	// ("assistant" | "tool" | "error" | "user") sin que tenga
	// que parsear el HTML. El cliente lo usa para saber si un
	// fragment es del assistant (y por lo tanto se aplica el
	// timer heurístico corto) o de un tool (donde el turno aún
	// sigue abierto aunque haya silencio entre chunks).
	ItemKind string `json:"itemKind,omitempty"`
	// QueueSteering y QueueFollowUp se envían en envelopes
	// kind="queue" cuando pi emite un queue_update. El cliente
	// los usa para el badge secundario "En cola: N" sin tocar
	// el phase primario (anti-flicker).
	QueueSteering int `json:"queueSteering,omitempty"`
	QueueFollowUp int `json:"queueFollowUp,omitempty"`
	// ClearAfterSeq se envía en envelopes kind="regenerate".
	// Indica al cliente que debe borrar del feed los items con
	// data-msg > ClearAfterSeq porque el server va a regenerar
	// la respuesta desde ese punto. Los nuevos fragments del
	// assistant llegan con seqs nuevos que se renderizan
	// normalmente.
	ClearAfterSeq uint64 `json:"clearAfterSeq,omitempty"`
	// UpsertKey es un ID alternativo y estable que el cliente
	// usa para upsert en lugar de `id`. Cuando está presente,
	// el cliente busca primero por `[data-upsert-key="X"]` y
	// sólo si no encuentra, cae a `[data-msg="N"]`. Lo usa el
	// streaming de `tool_execution_update`: una misma tool call
	// emite múltiples updates, cada uno con un seq de journal
	// distinto. Sin UpsertKey, el cliente trataría cada update
	// como un item NUEVO y appendearía N cards. Con UpsertKey
	// (igual a toolCallId), todos los updates reemplazan el
	// mismo nodo DOM, que es lo que el usuario espera.
	UpsertKey string `json:"upsertKey,omitempty"`
}

// FragmentRenderer pinta un ConversationItem a HTML. La UI del
// agente lo satisface apuntando al mismo RenderMessage que ya
// produce el feed inicial — así cliente y server usan la misma
// forma de DOM, y el cliente solo hace upsert por id sin distinguir
// "este vino de la página" vs "este vino de SSE".
type FragmentRenderer interface {
	RenderFragment(item ConversationItem) (string, error)
	// RenderToolResultPartial pinta el output parcial de un tool
	// (de tool_execution_update) sin pasar por el transcript. El
	// HTML producido debe incluir data-upsert-key="<toolCallID>"
	// además del data-msg habitual, para que el cliente pueda
	// reemplazar el mismo nodo DOM en cada update de la misma
	// tool call. El cliente también usa UpsertKey del envelope
	// para identificar el match (no el data-msg, que cambia
	// por evento).
	RenderToolResultPartial(toolCallID, toolName, text string) (string, error)
}

// registry agrupa el renderer fallback (opcional, configurado
// vía SetFragmentRenderer al boot del proceso) y los renderers
// por sesión (configurados vía SetSessionRenderer cuando una
// página del chat se sirve). El lookup es por sessionID: si hay
// un renderer registrado para esa sesión gana; si no, cae al
// fallback. Si ninguno está configurado, el handler SSE emite el
// envelope crudo del provider.
//
// Concurrencia: el SSE handler, el page handler y los tests pueden
// leer/escribir a la vez. sync.RWMutex protege el mapa y el
// fallback. El lookup es O(1) y nunca bloquea más allá de un RLock.
type registry struct {
	mu        sync.RWMutex
	fallback  FragmentRenderer
	perScope  map[string]FragmentRenderer
}

var globalRegistry = &registry{perScope: map[string]FragmentRenderer{}}

// SetFragmentRenderer inyecta el renderer fallback. Es opcional:
// el page handler de cada sesión registra su propio renderer vía
// SetSessionRenderer antes de servir la página, así que el
// fallback sólo aplica si el cliente abre un SSE sin haber pasado
// por la página (caso raro, típicamente dev-tools). Si nunca se
// setea, RenderFragment devuelve false y el handler SSE emite el
// envelope crudo del provider sin re-renderizar.
func SetFragmentRenderer(r FragmentRenderer) {
	globalRegistry.mu.Lock()
	globalRegistry.fallback = r
	globalRegistry.mu.Unlock()
}

// SetSessionRenderer registra un renderer específico para una
// sesión. El page handler lo llama antes de servir la página, de
// modo que el SSE de esa sesión emita HTML en el formato que el
// cliente ya está esperando. Múltiples sesiones pueden tener
// renderers distintos; el último registro para una misma sesión
// gana (útil para dev donde se pueden alternar formats en runtime).
func SetSessionRenderer(sessionID string, r FragmentRenderer) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || r == nil {
		return
	}
	globalRegistry.mu.Lock()
	globalRegistry.perScope[sessionID] = r
	globalRegistry.mu.Unlock()
}

// ClearSessionRenderer libera el renderer registrado para una
// sesión. Útil cuando la sesión se elimina (Delete) o cuando el
// host quiere volver al fallback para una sesión puntual.
func ClearSessionRenderer(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	globalRegistry.mu.Lock()
	delete(globalRegistry.perScope, sessionID)
	globalRegistry.mu.Unlock()
}

// rendererFor devuelve el renderer para una sesión: per-session si
// hay uno registrado, si no el fallback. El resultado puede ser nil
// si ninguno está configurado.
func rendererFor(sessionID string) FragmentRenderer {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	if r, ok := globalRegistry.perScope[sessionID]; ok {
		return r
	}
	return globalRegistry.fallback
}

// RendererFor es la versión exportada de rendererFor. La usan los
// tests E2E para asertar que el registry per-session quedó
// configurado por el flow del handler (sin tener que reproducir el
// flujo SSE completo). En producción los handlers siempre la
// consultan vía rendererFor internamente.
func RendererFor(sessionID string) FragmentRenderer {
	return rendererFor(sessionID)
}

// RenderFragment materializa el evento del provider en un evento
// del cliente. Devuelve false si el evento no produce UI (p.ej.
// un heartbeat del provider, o un evento de metadata interna).
func RenderFragment(sessionID string, seq uint64, event Event) (FragmentEvent, bool) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return FragmentEvent{}, false
	}
	// Los items terminal los emite MaterializeEvent (que ya
	// appendTranscriptItem). Acá sólo decidimos qué señal de fase
	// devolver. Para text_delta: emitimos un fragmento de
	// streaming con el texto acumulado en el preview del manager.
	switch event.Type {
	case "message_start":
		return FragmentEvent{Kind: "phase", Phase: "thinking"}, true
	case "message_update":
		return streamOrSkip(seq, sessionID, event)
	case "tool_execution_start":
		return FragmentEvent{Kind: "phase", Phase: "tooling"}, true
	case "tool_execution_update":
		// ponytail: pi emite `tool_execution_update` con
		// partialResult.content que es el output ACUMULADO hasta
		// el momento (no el delta). El cliente debe reemplazar el
		// mismo nodo DOM en cada update (upsert por toolCallId)
		// para que la card de output "crezca" en vivo, igual que
		// en el terminal. Sin este case, el cliente ve la card
		// de la tool (args) y un silencio hasta el END, dando la
		// sensación de que el agente se quedó pegado — lo que el
		// user reportó como "el chat pierde mensajes" durante
		// operaciones largas (npm install, cargo build, etc).
		//
		// El item NO se materializa al transcript: las partials
		// son transient. Sólo tool_execution_end persiste el
		// output final en tmp/agent-transcripts.
		return streamToolResultUpdate(sessionID, event)
	case "tool_execution_end":
		// ponytail: el output de la tool. Renderizamos el último
		// item del transcript que matchee este seq. Como
		// MaterializeEvent ya lo appendeó, debe estar disponible.
		// Si por alguna razón no está (race entre journal flush y
		// transcript write), devolvemos false y el SSE handler cae
		// al fallback de EmitFragment.
		//
		// Además: le pasamos el toolCallId del payload al render
		// para que el HTML incluya data-upsert-key="<toolCallId>"
		// y el cliente reemplace la card de streaming (que
		// compartimos en el mismo nodo DOM vía upsertKey).
		return emitToolResultEnd(sessionID, seq, event)
	case "runtime_error", "stderr":
		return FragmentEvent{Kind: "phase", Phase: "error"}, true
	case "turn_end", "agent_end", "agent_settled", "runtime_exit":
		return FragmentEvent{Kind: "turn-end"}, true
	// ponytail: pi emite fases adicionales que antes se
	// descartaban (caían al default). Sin mapeo, el cliente V2
	// recibe un phase desconocido, safePhase() lo manda a IDLE, y
	// el badge "Trabajando" parpadea a vacío durante una
	// compactación o un retry automático. Mapeamos los phases
	// que importan para el badge y dejamos que el badge secundario
	// los muestre como detalle cuando el toggle esté activo.
	case "compaction_start":
		return FragmentEvent{Kind: "phase", Phase: "compacting"}, true
	case "auto_retry_start":
		return FragmentEvent{Kind: "phase", Phase: "retrying"}, true
	// ponytail: turn_start llega apenas arranca un turno,
	// antes del primer message_start. Es redundante con
	// message_start (que también emite "thinking"), pero si
	// alguien en el futuro usa turn_start sin message_start
	// inmediato, el badge ya queda en thinking. Idempotente:
	// safePhase en el cliente ignora re-emisiones del mismo
	// estado.
	case "turn_start":
		return FragmentEvent{Kind: "phase", Phase: "thinking"}, true
	// ponytail: queue_update NO es un phase sino metadata
	// (cantidad de mensajes en cola steering/follow-up). Lo
	// emitimos como un envelope de tipo "queue" para que el
	// cliente actualice el badge secundario sin tocar el phase
	// primario. Importante: el badge primario sigue su
	// máquina de states habitual, este evento es aditivo.
	case "queue_update":
		return queueUpdateFragment(event), true
	default:
		return FragmentEvent{}, false
	}
}

// queueUpdateFragment construye un envelope de cola con el
// conteo de mensajes pendientes (steering + followUp). El cliente
// usa este conteo para mostrar el badge "En cola: N" cuando
// corresponde.
func queueUpdateFragment(event Event) FragmentEvent {
	var payload struct {
		Steering []any `json:"steering"`
		FollowUp []any `json:"followUp"`
	}
	if len(event.Payload) == 0 {
		return FragmentEvent{Kind: "queue", QueueSteering: 0, QueueFollowUp: 0}
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return FragmentEvent{Kind: "queue", QueueSteering: 0, QueueFollowUp: 0}
	}
	return FragmentEvent{
		Kind:          "queue",
		QueueSteering: len(payload.Steering),
		QueueFollowUp: len(payload.FollowUp),
	}
}

// emitLastItemForSeq busca el item del transcript con Seq == seq
// y lo renderiza con el itemKind que se le indique. Es helper
// para tool_execution_end: evita depender de que EmitFragment
// re-lea el disco después del write (puede haber un delta
// microscópico entre Write y read en algunos filesystems).
func emitLastItemForSeq(sessionID string, seq uint64, itemKind string) (FragmentEvent, bool) {
	renderer := rendererFor(sessionID)
	if renderer == nil {
		return FragmentEvent{}, false
	}
	history, err := LoadConversationHistory(sessionID, 0, 50)
	if err != nil || len(history.Items) == 0 {
		return FragmentEvent{}, false
	}
	for i := len(history.Items) - 1; i >= 0; i-- {
		if history.Items[i].Seq == seq {
			html, err := renderer.RenderFragment(history.Items[i])
			if err != nil || strings.TrimSpace(html) == "" {
				return FragmentEvent{}, false
			}
			return FragmentEvent{Kind: "fragment", ID: seq, HTML: html, ItemKind: itemKind}, true
		}
	}
	return FragmentEvent{}, false
}

// streamToolResultUpdate maneja tool_execution_update, donde
// pi envía el output acumulado hasta el momento (no el delta).
// Renderizamos un tool_result item con el texto parcial y lo
// emitimos con UpsertKey = toolCallId, para que el cliente
// reemplace el MISMO nodo DOM en cada update. Sin esto, una
// tool con N updates crearía N cards separadas (memoria y UX
// feos); con esto, la card "crece" en vivo como en el
// terminal.
//
// NOTA: el item NO se materializa al transcript. Las partials
// son transient — sólo el output final del tool_execution_end
// persiste en tmp/agent-transcripts. El transcript NO tiene
// conocimiento de los updates intermedios, lo cual es
// consistente con la semántica de "el output del tool es una
// unidad, no una serie de snapshots".
func streamToolResultUpdate(sessionID string, event Event) (FragmentEvent, bool) {
	renderer := rendererFor(sessionID)
	if renderer == nil {
		return FragmentEvent{}, false
	}
	partial, ok := extractToolPartialResult(event.Payload)
	if !ok {
		return FragmentEvent{}, false
	}
	if strings.TrimSpace(partial.toolCallID) == "" {
		return FragmentEvent{}, false
	}
	if strings.TrimSpace(partial.text) == "" {
		return FragmentEvent{}, false
	}
	// Renderizamos vía el mismo path que un item tool_result
	// transcript, con un ConversationItem sintético (no se
	// persiste). El renderer delega en la UI V2, que produce
	// el mismo HTML que para el END, con la salvedad de que
	// acá le pedimos que incluya data-upsert-key="<toolCallId>"
	// en el wrapper para que el upsert por stream funcione.
	html, err := renderer.RenderToolResultPartial(partial.toolCallID, partial.toolName, partial.text)
	if err != nil || strings.TrimSpace(html) == "" {
		return FragmentEvent{}, false
	}
	return FragmentEvent{
		Kind:      "fragment",
		ID:        0, // client usa UpsertKey, no el seq
		UpsertKey: partial.toolCallID,
		HTML:      html,
		ItemKind:  "tool_result",
	}, true
}

// emitToolResultEnd maneja tool_execution_end. A diferencia de
// emitLastItemForSeq, acá también queremos que el HTML lleve
// data-upsert-key="<toolCallId>" para que el cliente reemplace
// la card de streaming (que compartió nodo DOM con esta end).
// Si emitLastItemForSeq ya pintó la card por su cuenta sin
// upsertKey, el cliente crearía una segunda card con el mismo
// contenido cuando llegue el end. Peor: si los updates
// intermedios ya pintaron la card via streamToolResultUpdate,
// el end event llega y upsert por data-msg del end NO matchea
// la card de streaming (data-msg distinto) → el cliente
// appendearía una SEGUNDA card duplicada.
//
// Por eso extraemos el toolCallId del payload y lo pasamos
// tanto al UpsertKey del envelope como al render (via
// ConversationItem.ToolCallID) para que el HTML lleve
// data-upsert-key.
func emitToolResultEnd(sessionID string, seq uint64, event Event) (FragmentEvent, bool) {
	renderer := rendererFor(sessionID)
	if renderer == nil {
		return FragmentEvent{}, false
	}
	// Extraemos el toolCallId del payload para el UpsertKey.
	toolCallID, _ := extractToolCallIDFromPayload(event.Payload)
	// MaterializeEvent ya appendeó el item tool_result al
	// transcript. Lo buscamos por seq (mismo path que antes).
	history, err := LoadConversationHistory(sessionID, 0, 50)
	if err != nil || len(history.Items) == 0 {
		return FragmentEvent{}, false
	}
	for i := len(history.Items) - 1; i >= 0; i-- {
		if history.Items[i].Seq == seq && history.Items[i].Kind == "tool_result" {
			html, err := renderer.RenderFragment(history.Items[i])
			if err != nil || strings.TrimSpace(html) == "" {
				return FragmentEvent{}, false
			}
			return FragmentEvent{
				Kind:      "fragment",
				ID:        seq,
				UpsertKey: toolCallID,
				HTML:      html,
				ItemKind:  "tool_result",
			}, true
		}
	}
	return FragmentEvent{}, false
}

// toolPartial es la forma normalizada de un tool result parcial
// (de tool_execution_update) o final (de tool_execution_end). Lo
// usan streamToolResultUpdate y emitToolResultEnd.
type toolPartial struct {
	toolCallID string
	toolName   string
	text       string
}

// extractToolPartialResult extrae toolCallId, toolName y el
// texto acumulado del payload. La estructura del payload
// difiere entre tool_execution_update y tool_execution_end:
//
//   tool_execution_update: { toolCallId, toolName, args, partialResult: { content, details } }
//   tool_execution_end:    { toolCallId, toolName, args, result:         { content, details }, isError }
//
// Devuelve false si el payload no matchea ninguna de las dos
// formas o si el texto queda vacío.
func extractToolPartialResult(payload json.RawMessage) (toolPartial, bool) {
	var p toolPartial
	// Probamos primero el shape de update, luego el de end.
	if err := tryExtractToolPartial(payload, &p); err != nil {
		return toolPartial{}, false
	}
	return p, true
}

func tryExtractToolPartial(payload json.RawMessage, p *toolPartial) error {
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type resultEnvelope struct {
		Content []contentBlock `json:"content"`
		Details struct {
			Truncated      bool   `json:"truncation"`
			FullOutputPath string `json:"fullOutputPath"`
		} `json:"details"`
	}
	var envelope struct {
		ToolCallID    string         `json:"toolCallId"`
		ToolName      string         `json:"toolName"`
		PartialResult resultEnvelope `json:"partialResult"`
		Result        resultEnvelope `json:"result"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}
	p.toolCallID = strings.TrimSpace(envelope.ToolCallID)
	p.toolName = strings.TrimSpace(envelope.ToolName)
	// Preferimos partialResult (update); si no, caemos a result (end).
	src := envelope.PartialResult
	if len(src.Content) == 0 {
		src = envelope.Result
	}
	var b strings.Builder
	for _, c := range src.Content {
		if c.Type == "text" && c.Text != "" {
			b.WriteString(c.Text)
		}
	}
	text := b.String()
	if src.Details.Truncated && strings.TrimSpace(src.Details.FullOutputPath) != "" {
		text = text + "\n\n[output truncated — full output: " + src.Details.FullOutputPath + "]"
	}
	p.text = text
	return nil
}

// extractToolCallIDFromPayload extrae sólo el toolCallId del
// payload (de update o end). Lo usa emitToolResultEnd para
// poblar el UpsertKey sin tener que parsear el resto.
func extractToolCallIDFromPayload(payload json.RawMessage) (string, error) {
	var envelope struct {
		ToolCallID string `json:"toolCallId"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", err
	}
	return strings.TrimSpace(envelope.ToolCallID), nil
}

// streamOrSkip despacha eventos message_update del provider.
//   text_delta     → fragment del assistant (burbuja principal)
//   thinking_delta → fragment del thinking (bloque colapsable)
//   otros tipos     → descartados (toolcall_start/end, etc.)
//
// ponytail: el id del fragmento streaming es el Seq del
// ConversationItem en vuelo (lo mantiene transcriptState). El
// cliente debe upsertar por Seq para que la "burbuja de streaming"
// aparezca UNA vez y crezca con cada delta. Si dos items
// consecutivos tienen el mismo Seq el cliente solo ve el último —
// eso es lo que queremos: el delta reemplaza al anterior.
func streamOrSkip(seq uint64, sessionID string, event Event) (FragmentEvent, bool) {
	renderer := rendererFor(sessionID)
	if renderer == nil {
		return FragmentEvent{}, false
	}
	// ponytail: ver extractMessageUpdateDelta en history.go —
	// pi emite los text_deltas con el assistantMessageEvent
	// anidado bajo event.Payload.payload. Esta función acepta
	// ambos formatos para no perder los fragments de streaming
	// cuando el formato runtime cambia.
	deltaType, deltaText := extractMessageUpdateDelta(event.Payload)
	if deltaType == "" {
		return FragmentEvent{}, false
	}
	switch deltaType {
	case "text_delta":
		return streamDraftDelta(seq, sessionID, &transcriptState.assistant, "assistant", deltaText, renderer)
	case "thinking_delta":
		return streamDraftDelta(seq, sessionID, &transcriptState.thinking, "thinking", deltaText, renderer)
	}
	return FragmentEvent{}, false
}

// streamDraftDelta es el helper compartido por text_delta y
// thinking_delta. Mantiene el draft en el mapa que se le pasa
// (assistant o thinking), asigna Seq defensivamente si el draft
// todavía no fue inicializado por message_start, acumula el
// delta con mergeAssistantDelta y emite un FragmentEvent con el
// ItemKind que se le indique. Devuelve (zero, false) si el
// renderer falla o el HTML queda vacío.
func streamDraftDelta(
	seq uint64,
	sessionID string,
	drafts *map[string]ConversationItem,
	kind string,
	delta string,
	renderer FragmentRenderer,
) (FragmentEvent, bool) {
	if delta == "" {
		return FragmentEvent{}, false
	}
	transcriptState.Lock()
	item := (*drafts)[sessionID]
	if item.Seq == 0 {
		// ponytail: el draft se inicializa con Seq en
		// message_start (history.go). Defensa adicional: si
		// llega un delta sin draft previo (provider saltea
		// message_start), le asignamos el Seq de ESTE evento
		// para que el upsert por data-msg funcione. Antes
		// descartábamos el primer delta y la respuesta
		// aparecía toda junta sólo con message_end, sin
		// streaming visible.
		item.Seq = seq
		item.Kind = kind
	}
	item.Text = mergeAssistantDelta(item.Text, delta)
	(*drafts)[sessionID] = item
	transcriptState.Unlock()

	html, err := renderer.RenderFragment(item)
	if err != nil || strings.TrimSpace(html) == "" {
		return FragmentEvent{}, false
	}
	return FragmentEvent{Kind: "fragment", ID: item.Seq, HTML: html, ItemKind: kind}, true
}

// EmitFragment re-renderiza el último item del transcript (cuando
// MaterializeEvent lo materializó en message_end). El handler SSE
// lo llama tras la materialización para empujar el HTML final al
// cliente. Si no hay item nuevo desde la última emisión, devuelve
// false (idempotencia ante replays del journal).
func EmitFragment(sessionID string, seq uint64) (FragmentEvent, bool) {
	renderer := rendererFor(sessionID)
	if renderer == nil {
		return FragmentEvent{}, false
	}
	history, err := LoadConversationHistory(sessionID, 0, 50)
	if err != nil || len(history.Items) == 0 {
		return FragmentEvent{}, false
	}
	// Buscamos el item con seq == el que recién se materializó.
	var item *ConversationItem
	for i := range history.Items {
		if history.Items[i].Seq == seq {
			item = &history.Items[i]
			break
		}
	}
	if item == nil {
		// Si no matchea por seq exacto (caso típico: fragment
		// streaming), devolvemos el último item assistant.
		for i := len(history.Items) - 1; i >= 0; i-- {
			if history.Items[i].Kind == "assistant" {
				item = &history.Items[i]
				break
			}
		}
	}
	if item == nil {
		return FragmentEvent{}, false
	}
	html, err := renderer.RenderFragment(*item)
	if err != nil {
		return FragmentEvent{}, false
	}
	return FragmentEvent{Kind: "fragment", ID: item.Seq, HTML: html, ItemKind: item.Kind}, true
}

// ponytail: errores específicos evitan que el handler tenga que
// comparar contra nil si el caller quiere fallback.
var ErrFragmentRendererMissing = errors.New("agent: fragment renderer not configured")
