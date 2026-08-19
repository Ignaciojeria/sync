package application

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

type ConversationHistory struct {
	Items      []ConversationItem `json:"items"`
	LastSeq    uint64             `json:"lastSeq"`
	NextBefore uint64             `json:"nextBefore,omitempty"`
	HasMore    bool               `json:"hasMore"`
}

type ConversationItem struct {
	Seq      uint64          `json:"seq"`
	Kind     string          `json:"kind"`
	Text     string          `json:"text,omitempty"`
	ToolName string          `json:"toolName,omitempty"`
	Args     json.RawMessage `json:"args,omitempty"`
}

// ponytail: el read path del chat usa exclusivamente el transcript
// interno (tmp/agent-transcripts) con auto-recovery desde el pi
// session file. Antes existía una capa PostgreSQL adicional
// (RuntimeEventsHistorySource) que guardaba los eventos crudos
// del SSE para regenerar la conversación con
// rebuildConversationFromRuntimeRecords. Esa capa duplicaba el
// journal local, agregaba latencia de DB en el hot path del
// streaming, y tenía bugs propios (e.g. payload anidado en
// tool_execution_end). El journal local + transcript + pi
// session file ya cubren los 3 casos (lectura normal, transcript
// corrupto, sesión no registrada).

var transcriptState = struct {
	sync.Mutex
	assistant map[string]ConversationItem
	thinking  map[string]ConversationItem
}{
	assistant: map[string]ConversationItem{},
	thinking:  map[string]ConversationItem{},
}

func LoadConversationHistory(sessionID string, before uint64, limit int) (ConversationHistory, error) {
	return LoadConversationHistoryCtx(context.Background(), sessionID, before, limit)
}

func LoadConversationHistoryCtx(ctx context.Context, sessionID string, before uint64, limit int) (ConversationHistory, error) {
	if strings.TrimSpace(sessionID) == "" {
		return ConversationHistory{}, nil
	}
	if limit <= 0 {
		limit = 30
	}
	// ponytail: el read path va directo al transcript interno.
	// Antes intentaba primero PostgreSQL (RuntimeEventsHistorySource)
	// y caía al transcript si la DB no tenía rows. Esa capa se
	// eliminó — ver comentario arriba de RuntimeEventRecord.
	items, err := readConversationTranscript(sessionID)
	if err != nil {
		return ConversationHistory{}, err
	}
	// ponytail: fallback al pi session file. El transcript
	// interno (tmp/agent-transcripts) puede estar incompleto si
	// el SSE handler no capturó los eventos del assistant
	// (LRU eviction mata el runtime antes de que termine,
	// race entre message_end y turn_end, etc.). El pi session
	// file es la fuente de verdad de pi — pi escribe ahí
	// directo, independientemente del V2 server.
	//
	// Activamos el fallback si:
	//   - No hay items en el transcript (caso típico: el SSE
	//     handler nunca procesó nada).
	//   - Hay items pero ninguno es assistant/tool_result.
	//     Esto pasa cuando el DB tiene el user prompt pero
	//     perdió los eventos del assistant (común con LRU
	//     eviction que mata el runtime antes de message_end).
	//
	// Si el fallback trae más items, lo usamos. Si trae menos
	// o igual, mantenemos lo del DB (puede tener items del V1
	// que el pi session file no conoce).
	needsFallback := false
	if len(items) == 0 {
		needsFallback = true
	} else {
		hasAssistantOrTool := false
		for _, it := range items {
			if it.Kind == "assistant" || it.Kind == "tool_result" || it.Kind == "tool" {
				hasAssistantOrTool = true
				break
			}
		}
		if !hasAssistantOrTool {
			needsFallback = true
		}
	}
	if needsFallback {
		piItems := buildTranscriptFromPiSession(sessionID)
		if len(piItems) > len(items) {
			items = piItems
		}
	} else {
		// ponytail: detectar corrupción por mergeAssistantDelta.
		// Si el transcript tiene MISMOS items que el pi session
		// file pero el último assistant del transcript es
		// significativamente más corto que el del pi file,
		// significa que el transcript fue escrito durante el
		// streaming (cuando el merge aún corrompía) y nunca se
		// reescribió desde el payload completo de message_end.
		// En ese caso, preferimos el pi file como fuente de
		// verdad.
		//
		// Heurística: si el último assistant del transcript
		// tiene <70% del largo del último assistant del pi
		// file, lo damos por corrupto. El threshold de 70%
		// evita falsos positivos para resúmenes cortos
		// legítimos (ej. assistant que respondió "ok" — el pi
		// file también lo tiene corto).
		//
		// Además: si el count de assistant/tool_result items
		// del transcript supera al del pi file por un margen
		// grande (≥20%), también es señal de duplicación —
		// el transcript tiene entradas de más porque el
		// mergeAssistantDelta generó copias spurias. En ese
		// caso preferimos el pi file también.
		if piItems := buildTranscriptFromPiSession(sessionID); len(piItems) > 0 {
			if piLast, dbLast := lastAssistantText(piItems), lastAssistantText(items); piLast != "" && dbLast != "" {
				if len(dbLast)*10 < len(piLast)*7 {
					items = piItems
				}
			}
			if !corruptionAlreadyHandled(items, piItems) {
				if countByKind(piItems, "assistant") > 0 && countByKind(items, "assistant")*10 > countByKind(piItems, "assistant")*12 {
					items = piItems
				}
				if countByKind(piItems, "tool_result") > 0 && countByKind(items, "tool_result")*10 > countByKind(piItems, "tool_result")*12 {
					items = piItems
				}
			}
		}
	}
	if before > 0 {
		filtered := items[:0]
		for _, item := range items {
			if item.Seq < before {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if len(items) == 0 {
		return ConversationHistory{}, nil
	}
	out := ConversationHistory{LastSeq: items[len(items)-1].Seq}
	if len(items) > limit {
		out.HasMore = true
		out.NextBefore = items[len(items)-limit].Seq
		items = items[len(items)-limit:]
	}
	out.Items = items
	out.LastSeq = items[len(items)-1].Seq
	if before == 0 && limit == 1 && len(out.Items) == 1 {
		out.Items[0].Text = previewText(out.Items[0].Text, 1200)
	}
	return out, nil
}

// flushDraftsIfPending vuelca los drafts in-memory al transcript
// si tienen contenido, y los borra del map después. Es seguro
// llamarlo múltiples veces (la segunda vez es no-op si no hay
// drafts nuevos) y desde varios goroutines por sesión, porque la
// transición copy/delete va bajo el mismo lock que usa
// appendTranscriptItem.
//
// Por qué: si el runtime muere entre message_start y message_end
// (LRU eviction, abort a mitad de streaming, crash del proceso
// hijo), los drafts quedan atrapados en transcriptState y se
// evaporan con el proceso. El journal tiene los eventos crudos,
// pero el transcript queda incompleto para siempre. Flushar acá
// garantiza que cualquier texto parcial llegue al transcript,
// para que el próximo readConversationTranscript lo encuentre
// incluso si el siguiente message_end nunca llega.
//
// fallbackSeq se usa cuando el draft todavía no tiene Seq (caso
// típico: message_start sin ningún text_delta posterior). En ese
// caso adoptamos el Seq del evento que disparó el flush.
func flushDraftsIfPending(sessionID string, fallbackSeq uint64) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	transcriptState.Lock()
	assistant, hasAssistant := transcriptState.assistant[sessionID]
	thinking, hasThinking := transcriptState.thinking[sessionID]
	delete(transcriptState.assistant, sessionID)
	delete(transcriptState.thinking, sessionID)
	transcriptState.Unlock()
	if hasAssistant && strings.TrimSpace(assistant.Text) != "" {
		if assistant.Seq == 0 {
			assistant.Seq = fallbackSeq
		}
		_ = appendTranscriptItem(sessionID, assistant)
	}
	if hasThinking && strings.TrimSpace(thinking.Text) != "" {
		if thinking.Seq == 0 {
			thinking.Seq = fallbackSeq
		}
		_ = appendTranscriptItem(sessionID, thinking)
	}
}

// clearDrafts elimina los drafts in-memory de una sesión sin
// flushear a disco. Lo usa message_end (que ya escribió los items
// finales) y la rama corta de message_start cuando el payload
// trae un error terminal del assistant (en ese caso no queremos
// arrastrar un draft vacío al turno siguiente).
func clearDrafts(sessionID string) {
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	transcriptState.Lock()
	delete(transcriptState.assistant, sessionID)
	delete(transcriptState.thinking, sessionID)
	transcriptState.Unlock()
}

func MaterializeUserPrompt(sessionID, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	// ponytail: el nuevo user_prompt implícitamente cierra el turn
	// anterior del assistant (pi aborta el stream en curso cuando
	// recibe un nuevo prompt por stdin). Si el assistant estaba
	// mid-stream y algún draft con texto parcial quedaba en RAM,
	// sin este flush se perdería entre el message_start del nuevo
	// turn y el message_end (lo que pasaba antes de FIX #1). El
	// fix original cubría message_start con flushDraftsIfPending,
	// pero ese flush sólo corre cuando el runtime emite
	// message_start. Si el user corta el stream a mano (abort) o
	// pi nunca emite message_start tras el corte, los drafts
	// quedan huérfanos. Llamarlo acá sella esa ventana.
	flushDraftsIfPending(sessionID, 0)
	_ = appendTranscriptItem(sessionID, ConversationItem{Kind: "user", Text: text})
}

func MaterializeEvent(sessionID string, seq uint64, event Event) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	switch event.Type {
	case "message_start":
		// ponytail: antes de inicializar drafts nuevos, flushar
		// los del turno anterior. El bug clásico era: turn N
		// emite message_start + text_deltas pero message_end se
		// pierde (runtime LRU-evicted, abort, etc.); al llegar el
		// message_start del turn N+1 el código pisaba los drafts
		// de N sin flushear, perdiendo el texto del assistant.
		flushDraftsIfPending(sessionID, seq)
		if text, ok := extractAssistantStopError(event.Payload); ok {
			_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "error", Text: text})
			return
		}
		// ponytail: inicializamos AMBOS drafts (assistant +
		// thinking). Pi emite un solo message_start por mensaje
		// del assistant que puede contener tanto razonamiento
		// como respuesta. Si no inicializamos el thinking acá,
		// el primer thinking_delta se queda con Seq=0 y entra al
		// fallback de streamDraftDelta (que igual lo rescata),
		// pero el transcript no queda consistente para replay.
		transcriptState.Lock()
		transcriptState.assistant[sessionID] = ConversationItem{Kind: "assistant"}
		transcriptState.thinking[sessionID] = ConversationItem{Kind: "thinking"}
		transcriptState.Unlock()
	case "message_update":
		// ponytail: pi emite text_deltas con una estructura
		// anidada: assistantMessageEvent vive bajo
		// event.Payload.payload (no directamente en
		// event.Payload). Vimos esto en
		// agent-1784783206892149281-1 donde el código previo con
		// solo el path plano leía Type="" / Delta="" y los drafts
		// no se actualizaban — el SSE handler igual mandaba los
		// fragments al cliente (vía streamDraftDelta en
		// RenderFragment) pero el transcript file quedaba
		// vacío para text_delta, y al message_end sólo se
		// flusheaba el payload — los items reconstruidos del
		// journal tenían mergeAssistantDelta corrupto
		// (overlap=1 bug). extractMessageUpdateDelta extrae
		// (type, delta) de CUALQUIERA de las dos formas.
		deltaType, deltaText := extractMessageUpdateDelta(event.Payload)
		if deltaType == "" {
			return
		}
		transcriptState.Lock()
		switch deltaType {
		case "text_delta":
			item := transcriptState.assistant[sessionID]
			item.Kind = "assistant"
			if item.Seq == 0 {
				item.Seq = seq
			}
			item.Text = mergeAssistantDelta(item.Text, deltaText)
			transcriptState.assistant[sessionID] = item
		case "thinking_delta":
			item := transcriptState.thinking[sessionID]
			item.Kind = "thinking"
			if item.Seq == 0 {
				item.Seq = seq
			}
			item.Text = mergeAssistantDelta(item.Text, deltaText)
			transcriptState.thinking[sessionID] = item
		}
		transcriptState.Unlock()
	case "message_end":
		// Flush de AMBOS drafts. Si thinking quedó vacío
		// (preguntas simples sin razonamiento), NO lo
		// materializamos para no ensuciar el feed con un item
		// vacío.
		transcriptState.Lock()
		assistant, hasAssistant := transcriptState.assistant[sessionID]
		thinking, hasThinking := transcriptState.thinking[sessionID]
		delete(transcriptState.assistant, sessionID)
		delete(transcriptState.thinking, sessionID)
		transcriptState.Unlock()
		// ponytail: preferir el texto completo que viene en el
		// payload del evento message_end sobre el draft
		// acumulado por text_delta. Pi envía message.content
		// con el contenido final del assistant — es la fuente
		// de verdad y NO tiene la corrupción que pueda haber
		// metido mergeAssistantDelta durante el streaming.
		//
		// Esto resuelve el bug donde textos largos con muchos
		// espacios al borde o palabras repetidas perdían chars
		// en el acumulado incremental (overlap=1 entre
		// espacios consecutivos, etc).
		//
		// CRÍTICO: pi emite message_end también para mensajes
		// con role=toolResult / role=user (en particular, el
		// tool result llega dos veces: una como
		// tool_execution_end y otra como message_end con
		// role=toolResult). Si dejamos el draft tal cual
		// cuando el role no es assistant, persistimos texto
		// stale del turno anterior como si fuera del
		// assistant. Por eso extractAssistantContentFromPayload
		// ahora chequea role y, cuando no es assistant,
		// también vaciamos el draft explícitamente.
		payloadText, payloadThinking, payloadHasMessage, payloadRole := extractAssistantContentFromPayload(event.Payload)
		payloadIsAssistant := payloadHasMessage && strings.EqualFold(strings.TrimSpace(payloadRole), "assistant")
		if hasAssistant {
			if payloadIsAssistant && payloadText != "" {
				assistant.Text = payloadText
			} else if payloadHasMessage && !payloadIsAssistant {
				// Payload presente con role distinto de
				// assistant (típicamente role=toolResult, que
				// pi emite después de tool_execution_end). El
				// draft puede tener contenido stale del turno
				// anterior — descartarlo para no duplicar el
				// tool_result.
				assistant.Text = ""
			}
			// Si !payloadHasMessage (payload ausente o shape
			// desconocido): caemos al draft como antes.
		}
		if hasThinking {
			if payloadIsAssistant && payloadThinking != "" {
				thinking.Text = payloadThinking
			} else if payloadHasMessage && !payloadIsAssistant {
				thinking.Text = ""
			}
		}
		if hasThinking && strings.TrimSpace(thinking.Text) != "" {
			if thinking.Seq == 0 {
				thinking.Seq = seq
			}
			_ = appendTranscriptItem(sessionID, thinking)
		}
		if hasAssistant && strings.TrimSpace(assistant.Text) != "" {
			if assistant.Seq == 0 {
				assistant.Seq = seq
			}
			_ = appendTranscriptItem(sessionID, assistant)
		}
	case "tool_execution_start":
		var payload struct {
			ToolName string          `json:"toolName"`
			Type     string          `json:"type"`
			Args     json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		name := strings.TrimSpace(payload.ToolName)
		if name == "" {
			name = strings.TrimSpace(payload.Type)
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "tool", ToolName: name, Args: payload.Args})
	case "tool_execution_end":
		// ponytail: antes este evento se descartaba, así que el
		// chat solo mostraba los ARGS de la tool (ej. "bash: ls
		// -la") pero nunca el RESULTADO. Acá extraemos el
		// contenido de result.content[0].text (la estructura que
		// emite pi para tools de texto) y lo materializamos como
		// un item aparte con kind="tool_result". El cliente V2
		// renderiza tool + tool_result como cards adyacentes
		// (args arriba, output abajo).
		var payload struct {
			ToolName string `json:"toolName"`
			Result   struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				Details struct {
					Truncated     bool   `json:"truncation"`
					FullOutputPath string `json:"fullOutputPath"`
				} `json:"details"`
			} `json:"result"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		var output strings.Builder
		for _, c := range payload.Result.Content {
			if c.Type == "text" && c.Text != "" {
				output.WriteString(c.Text)
			}
		}
		text := output.String()
		if text == "" {
			return
		}
		if payload.Result.Details.Truncated && strings.TrimSpace(payload.Result.Details.FullOutputPath) != "" {
			text = text + "\n\n[output truncated — full output: " + payload.Result.Details.FullOutputPath + "]"
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{
			Seq:      seq,
			Kind:     "tool_result",
			ToolName: strings.TrimSpace(payload.ToolName),
			Text:     text,
		})
	case "turn_end":
		// ponytail: el bug "message_end missed but turn_end came"
		// lo cubrimos acá. message_end borra los drafts (ver
		// más abajo) — si llegó primero, este case es no-op.
		// Si NO llegó message_end, los drafts siguen con texto
		// parcial; los flushar antes de devolver evita la
		// pérdida silenciosa.
		flushDraftsIfPending(sessionID, seq)
	case "agent_end":
		// idem turn_end — el runtime está terminando toda la
		// sesión; cualquier draft pendiente debe persistirse.
		flushDraftsIfPending(sessionID, seq)
	case "agent_settled":
		// agent_settled es la señal "estable" que pi emite antes
		// de agent_end en algunas versiones; tratamos igual que
		// turn_end por seguridad.
		flushDraftsIfPending(sessionID, seq)
	case "runtime_exit":
		// Definitive termination. Flush antes de escribir el
		// error item — el journal ya tiene los eventos crudos
		// pero el transcript puede haber quedado parcial.
		flushDraftsIfPending(sessionID, seq)
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
		if text == "" {
			text = event.Type
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "error", Text: text})
	case "runtime_error", "stderr":
		// No flushamos acá: runtime_error puede ser transitorio
		// y el runtime puede seguir emitiendo más eventos (otro
		// message_start que reinicializa drafts). Si fuera
		// terminal, va a llegar un runtime_exit que sí flushea.
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
		if text == "" {
			text = event.Type
		}
		_ = appendTranscriptItem(sessionID, ConversationItem{Seq: seq, Kind: "error", Text: text})
	}
}

func readConversationTranscript(sessionID string) ([]ConversationItem, error) {
	path := transcriptPath(sessionID)
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		items, backfillErr := buildTranscriptFromLegacyJournal(sessionID)
		if backfillErr != nil {
			return nil, backfillErr
		}
		if len(items) > 0 {
			_ = rewriteTranscript(sessionID, items)
		}
		return dedupItems(items), nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []ConversationItem
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var item ConversationItem
		if err := json.Unmarshal(sc.Bytes(), &item); err == nil && strings.TrimSpace(item.Kind) != "" {
			out = append(out, item)
		}
	}
	// ponytail: FIX #2 rediseñado. La versión anterior hacía
	// rebuild agresivo desde el journal cuando journalLast >
	// transcriptLast+5. Eso causaba bugs en sesiones activas
	// porque el rebuild usa mergeAssistantDelta (con bug de
	// overlap=1 conocido) y pisa drafts que el SSE handler
	// acaba de flushear. Secuelas observadas en
	// agent-1784783206892149281-1:
	//
	//   - seq=717 assistant aparecía dos veces en el transcript:
	//     una versión "rebuild'd" trunca a 1422 chars y la
	//     otra "live" completa con 3571 chars.
	//   - El rebuild asignaba current.Seq=0 al último assistant
	//     porque las message_starts estaban anidadas en
	//     `event.Payload.payload` (estructura que la rutina de
	//     rebuild no manejaba correctamente) — la versión Live
	//     MaterializeEvent sí las maneja.
	//   - Concurrent builds racing con el SSE handler
	//     escribían versiones inconsistentes del transcript.
	//
	// Estrategia nueva: NO rebuildar sobre un transcript que ya
	// tiene contenido. Si hay gap (journal muy adelante), lo
	// cerramos via FIX #1: la próxima vez que message_start /
	// turn_end / runtime_exit se dispare, flushDraftsIfPending
	// vuelca los drafts pendientes al transcript. Si la sesión
	// se cerró abruptamente, el operator puede correr
	// `cmd/rebuild-transcript` (script manual) para recovery.
	// El journal sigue siendo source of truth para replay
	// manual; el transcript es para fast read.
	//
	// Si el transcript está vacío (caso fresh start), el
	// rebuild desde el journal sigue siendo la única forma de
	// poblarlo — eso se maneja arriba con `os.ErrNotExist`.
	return dedupItems(out), sc.Err()
}

// dedupItems elimina duplicados consecutivos EXACTAMENTE iguales
// (misma seq, kind, texto). El viejo criterio "mismo (seq,kind)"
// borraba user_prompts porque todos tienen Seq=0 por diseño
// (MaterializeUserPrompt no asigna seq: cada prompt es un
// evento previo al flujo SSE/journal). En la sesión del user
// agent-1784784744615003050-1, había 7 user prompts en el
// transcript — todos con Seq=0 — y el dedup viejo dejaba sólo
// el primero. Por eso reportó "se pierden mis mensajes más que
// la respuesta del agente": la respuesta del assistant tiene
// seqs únicos (290, 651, etc.) y sobrevivía; los user prompts
// compartían Seq=0 y desaparecían del render.
//
// El nuevo criterio usa un hash corto (len+kind+seq+text[:40])
// que es bueno-en-gran medida: detecta líneas id duplas sin
// confundir user prompts distintos. Si dos user prompts tienen
// los primeros 40 chars idénticos Y el mismo long, el dedup los
// mergea — eso es aceptable porque ambos prompts serían
// idénticos en contenido (y por lo tanto bugs upstream, no
// pérdida de info).
func dedupItems(items []ConversationItem) []ConversationItem {
	if len(items) == 0 {
		return items
	}
	sig := func(it ConversationItem) string {
		text := it.Text
		if len(text) > 40 {
			text = text[:40]
		}
		return fmt.Sprintf("%d|%s|%s", len(it.Text), it.Kind, text)
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]ConversationItem, 0, len(items))
	for _, item := range items {
		key := sig(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func appendTranscriptItem(sessionID string, item ConversationItem) error {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(item.Kind) == "" {
		return nil
	}
	transcriptState.Lock()
	defer transcriptState.Unlock()
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		return err
	}
	body, err := json.Marshal(item)
	if err != nil {
		return err
	}
	// ponytail: dedup contra la última línea del transcript. Hay
	// varios paths que pueden llamar appendTranscriptItem con el
	// mismo item (MaterializeEvent en message_end + flushDraftsIfPending
	// en runtime_exit, FIX #2 rebuild que re-escribe desde el
	// journal, etc.). Cuando dos paths corren para el mismo draft
	// — por orden de eventos del provider (message_start llega
	// después de message_end, runtime_exit después de turn_end,
	// etc.) o por re-emisión del runtime con payloads ligeramente
	// distintos — el transcript termina con duplicados de la
	// misma (seq, kind). El user reportó esto en la sesión
	// agent-1784783206892149281-1: seq=290 assistant aparecía
	// dos veces con "cora tests" (draft bug) y "corra tests"
	// (payload corregido). El bug raíz lo causa la combinación
	// de varios paths; el fix es idempotencia: antes de appendear,
	// leer la última línea y skipear si matchea.
	path := transcriptPath(sessionID)
	if existing, err := readLastTranscriptLine(path); err == nil && bytes.Equal(existing, body) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(body, '\n')); err != nil {
		return err
	}
	// ponytail: forzamos fsync para que un LoadConversationHistory
	// inmediato (típico en el SSE handler que acaba de hacer
	// MaterializeEvent y ya está por emitir el fragment vía
	// emitLastItemForSeq) vea la línea recién escrita. Sin fsync
	// el OS puede batchear los writes en page cache y la lectura
	// desde el mismo proceso ve un archivo más corto.
	return f.Sync()
}

// readLastTranscriptLine devuelve la última línea no vacía del
// transcript file (sin newline final) o nil si está vacío o no
// existe. Sirve para el dedup de appendTranscriptItem. Limit a
// las últimas ~64KB para no escanear archivos grandes; en la
// práctica cada item es <16KB y el dedup solo necesita ver la
// última línea.
func readLastTranscriptLine(path string) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || stat.Size() == 0 {
		return nil, nil
	}
	const maxRead = 64 * 1024
	readSize := stat.Size()
	if readSize > maxRead {
		readSize = maxRead
	}
	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, stat.Size()-readSize); err != nil {
		// fallback: scan from start if ReadAt fails (shouldn't)
		if _, err := f.Seek(stat.Size()-readSize, 0); err != nil {
			return nil, err
		}
		if _, err := f.Read(buf); err != nil {
			return nil, err
		}
	}
	// Find the last newline in buf; everything after is the last
	// line. Edge case: appendTranscriptItem always writes a
	// trailing newline, so the file's last byte is `\n`. Walk back
	// through trailing newlines (handle empty lines and the
	// terminator) until we hit a non-newline; the chunk after the
	// preceding newline is the last non-empty line.
	lastNonNewline := len(buf) - 1
	for lastNonNewline >= 0 && buf[lastNonNewline] == '\n' {
		lastNonNewline--
	}
	if lastNonNewline < 0 {
		return nil, nil
	}
	// Find the newline that ends the last non-empty line.
	for i := lastNonNewline - 1; i >= 0; i-- {
		if buf[i] == '\n' {
			line := buf[i+1 : lastNonNewline+1]
			if len(line) == 0 {
				return nil, nil
			}
			return line, nil
		}
	}
	// No newline in buf (whole file is one line, no trailing \n).
	return buf[:lastNonNewline+1], nil
}

func rewriteTranscript(sessionID string, items []ConversationItem) error {
	transcriptState.Lock()
	defer transcriptState.Unlock()
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		return err
	}
	f, err := os.Create(transcriptPath(sessionID))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, item := range items {
		body, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := f.Write(append(body, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// ponytail: truncateTranscriptAfter elimina todos los items
// del transcript cuyo Seq es estrictamente mayor a afterSeq.
// Lo usa Regenerate para borrar las respuestas viejas del
// assistant antes de re-enviar el prompt. El cliente borrará
// los items correspondientes del DOM cuando reciba el envelope
// kind="regenerate" del SSE.
//
// Si afterSeq es 0, NO borra nada (es un no-op defensivo).
//
// IMPORTANTE: los items con Seq=0 son los del user prompt
// original del POST (MaterializeUserPrompt sin seq). El
// filter usa `Seq > 0` para NO incluirlos en el filtro
// (queremos mantenerlos). Pero también debemos asegurarnos
// de NO crear duplicados cuando se regenera — el nuevo
// user_prompt del SSE handler también tiene Seq=0. Para
// evitar eso, borramos items con `Seq == 0 && Kind == "user"`
// antes de filtrar por Seq.
//
// ponytail: el bug que el user reportó (pregunta desaparecida
// tras Regenerate) era porque el filter dejaba seq=0 viejo y
// seq=0 nuevo. La fix borra items con Seq=0 user antes de
// filtrar para que el nuevo user_prompt (seq=0) sea el único.
func truncateTranscriptAfter(sessionID string, afterSeq uint64) error {
	items, err := readConversationTranscript(sessionID)
	if err != nil {
		return err
	}
	if afterSeq == 0 {
		return nil
	}
	filtered := items[:0]
	for _, item := range items {
		// Skip old seq=0 user prompts — they'd duplicate the
		// new seq=0 user that the SSE handler will materialize
		// when Regenerate re-sends the prompt.
		if item.Seq == 0 && item.Kind == "user" {
			continue
		}
		if item.Seq > afterSeq || (item.Seq == 0 && item.Kind != "user") {
			continue
		}
		filtered = append(filtered, item)
	}
	return rewriteTranscript(sessionID, filtered)
}

type historyJournalEntry struct {
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func buildTranscriptFromLegacyJournal(sessionID string) ([]ConversationItem, error) {
	entries, err := readSessionJournal(sessionID)
	if err != nil {
		return nil, err
	}
	return rebuildConversation(entries), nil
}

// journalLastSeq devuelve el mayor Seq presente en el journal de
// la sesión, o 0 si el archivo no existe. Se usa en
// readConversationTranscript para detectar transcripts stale.
// Itera el archivo línea por línea (O(n) en bytes) pero las
// sesiones activas raramente superan los 100MB, y el scan es
// hot-path solo en page reload (no en SSE hot-loop).
func journalLastSeq(sessionID string) uint64 {
	sessionID = sanitizeSessionID(strings.TrimSpace(sessionID))
	if sessionID == "" {
		return 0
	}
	path := filepath.Join(eventsJournalDir(), sessionID+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var maxSeq uint64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e historyJournalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil && e.Seq > maxSeq {
			maxSeq = e.Seq
		}
	}
	_ = sc.Err()
	return maxSeq
}

func readSessionJournal(sessionID string) ([]historyJournalEntry, error) {
	path := filepath.Join(eventsJournalDir(), sanitizeSessionID(sessionID)+".jsonl")
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []historyJournalEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e historyJournalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

func rebuildConversation(entries []historyJournalEntry) []ConversationItem {
	items := make([]ConversationItem, 0, len(entries))
	current := ConversationItem{Kind: "assistant"}
	thinking := ConversationItem{Kind: "thinking"}
	flushAssistant := func() {
		if strings.TrimSpace(current.Text) != "" {
			items = append(items, current)
		}
		current = ConversationItem{Kind: "assistant"}
	}
	flushThinking := func() {
		if strings.TrimSpace(thinking.Text) != "" {
			items = append(items, thinking)
		}
		thinking = ConversationItem{Kind: "thinking"}
	}
	for _, entry := range entries {
		var event Event
		if err := json.Unmarshal(entry.Payload, &event); err != nil {
			continue
		}
		switch event.Type {
		case "user_prompt":
			flushAssistant()
			flushThinking()
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			text := strings.TrimSpace(payload.Text)
			if text == "" {
				continue
			}
			items = append(items, ConversationItem{Seq: entry.Seq, Kind: "user", Text: text})
		case "message_start":
			flushAssistant()
			flushThinking()
			if text, ok := extractAssistantStopError(event.Payload); ok {
				items = append(items, ConversationItem{Seq: entry.Seq, Kind: "error", Text: text})
			}
		case "message_update":
			// ponytail: usa el helper tolerante a formato (event.Payload
			// plano vs anidado). Vimos en agent-1784783206892149281-1
			// que pi emite la forma anidada — el Unmarshal top-level
			// fallaba y los text_deltas nunca llegaban al rebuild.
			deltaType, deltaText := extractMessageUpdateDelta(entry.Payload)
			switch deltaType {
			case "text_delta":
				current.Kind = "assistant"
				if current.Seq == 0 {
					current.Seq = entry.Seq
				}
				current.Text = mergeAssistantDelta(current.Text, deltaText)
			case "thinking_delta":
				thinking.Kind = "thinking"
				if thinking.Seq == 0 {
					thinking.Seq = entry.Seq
				}
				thinking.Text = mergeAssistantDelta(thinking.Text, deltaText)
			}
		case "message_end":
			// ponytail: preferir texto completo del payload del
			// evento sobre el draft acumulado por text_delta.
			// Pi envía el contenido final en message.content —
			// fuente de verdad sin la corrupción del merge
			// incremental. Si el payload no tiene content,
			// caemos al draft (lógica existente).
			if payloadText, payloadThinking, payloadHasMessage, payloadRole := extractAssistantContentFromPayload(event.Payload); payloadHasMessage && strings.EqualFold(strings.TrimSpace(payloadRole), "assistant") {
				if payloadText != "" {
					current.Text = payloadText
				}
				if payloadThinking != "" {
					thinking.Text = payloadThinking
				}
			}
			flushThinking()
			flushAssistant()
		case "tool_execution_start":
			flushAssistant()
			flushThinking()
			var payload struct {
				ToolName string          `json:"toolName"`
				Type     string          `json:"type"`
				Args     json.RawMessage `json:"args"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			name := strings.TrimSpace(payload.ToolName)
			if name == "" {
				name = strings.TrimSpace(payload.Type)
			}
			items = append(items, ConversationItem{Seq: entry.Seq, Kind: "tool", ToolName: name, Args: payload.Args})
		case "tool_execution_end":
			// ponytail: record.Payload es el event JSON ENTERO
			// (con la envoltura sessionId/type/payload). Los
			// campos de la tool (toolName, result) están
			// dentro del payload anidado, no al top level.
			// Hay que parsear event.Payload (ya extraído por
			// el switch de event.Type), NO record.Payload.
			// Sin esto, el rebuild pierde todos los tool_result
			// y la UI muestra la tool sin su output.
			var payload struct {
				ToolName string `json:"toolName"`
				Result   struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				} `json:"result"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			var output strings.Builder
			for _, c := range payload.Result.Content {
				if c.Type == "text" && c.Text != "" {
					output.WriteString(c.Text)
				}
			}
			text := output.String()
			if text == "" {
				continue
			}
			items = append(items, ConversationItem{
				Seq:      entry.Seq,
				Kind:     "tool_result",
				ToolName: strings.TrimSpace(payload.ToolName),
				Text:     text,
			})
		case "runtime_error", "stderr", "runtime_exit":
			flushAssistant()
			flushThinking()
			var payload struct {
				Message json.RawMessage `json:"message"`
				Reason  string          `json:"reason"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				continue
			}
			text := extractMessageText(payload.Message)
			if text == "" {
				text = strings.TrimSpace(payload.Reason)
			}
			if text == "" {
				text = event.Type
			}
			items = append(items, ConversationItem{Seq: entry.Seq, Kind: "error", Text: text})
		}
	}
	flushThinking()
	flushAssistant()
	return items
}

func ParseHistoryBefore(raw string) uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func ParseHistoryLimit(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 50 {
		n = 50
	}
	return n
}

func eventsJournalDir() string {
	dir := strings.TrimSpace(os.Getenv("AGENT_EVENTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-events"
	}
	return dir
}

func transcriptDir() string {
	dir := strings.TrimSpace(os.Getenv("AGENT_TRANSCRIPTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-transcripts"
	}
	return dir
}

func transcriptPath(sessionID string) string {
	return filepath.Join(transcriptDir(), sanitizeSessionID(sessionID)+".jsonl")
}

// piSessionPath devuelve el path al pi session file. Lo usa el
// fallback de buildTranscriptFromPiSession cuando el transcript
// interno está vacío.
//
// ponytail: el pi session file vive en
// AGENT_PI_SESSIONS_DIR (default: tmp/agent-pi-sessions). Es
// donde pi escribe directo, sin pasar por el V2 server. Es la
// fuente de verdad cuando el SSE handler falla (LRU eviction,
// race conditions, etc).
func piSessionPath(sessionID string) string {
	dir := strings.TrimSpace(os.Getenv("AGENT_PI_SESSIONS_DIR"))
	if dir == "" {
		dir = "tmp/agent-pi-sessions"
	}
	return filepath.Join(dir, sanitizeSessionID(sessionID)+".jsonl")
}

// buildTranscriptFromPiSession parsea el pi session file como
// fallback cuando el transcript interno está vacío. El formato
// del pi session file es JSONL con eventos:
//
//   {"type":"model_change",...}
//   {"type":"message","id":"...","message":{"role":"user",
//    "content":[{"type":"text","text":"..."}]}}
//   {"type":"message","id":"...","message":{"role":"assistant",
//    "content":[{"type":"thinking","thinking":"..."},
//              {"type":"text","text":"..."}]}}
//   {"type":"message","id":"...","message":{"role":"toolResult",
//    "toolCallId":"...","toolName":"...","content":[{...}]}}
//
// Convertimos esos mensajes a ConversationItem con:
//   - user → kind="user", text=concatenación de content.text
//   - assistant → kind="assistant", text=concatenación de
//     content.text (el thinking va a un item separado
//     kind="thinking" para que el render del V2 lo muestre
//     colapsado como bloque aparte)
//   - toolResult → kind="tool_result", text=concatenación de
//     content.text, toolName=message.toolName
//
// Si el archivo no existe o falla el parseo, retorna nil (no
// error). El caller ya tiene un fallback (transcript vacío).
func buildTranscriptFromPiSession(sessionID string) []ConversationItem {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	path := piSessionPath(sessionID)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var items []ConversationItem
	seq := uint64(0)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	// ponytail: el seq de cada item lo derivamos del índice del
	// mensaje en el pi session file (no del ID interno de pi,
	// que es un UUID no-mono). Cuando un mismo mensaje del
	// assistant produce DOS items (thinking + assistant), les
	// damos seqs consecutivos (N y N+1) para que el upsert del
	// cliente no los confunda — sin esto, ambos tendrían el
	// mismo seq y el upsert por data-msg los sobrescribiría.
	nextSeq := func() uint64 {
		seq++
		return seq
	}
	for sc.Scan() {
		var raw map[string]any
		if err := json.Unmarshal(sc.Bytes(), &raw); err != nil {
			continue
		}
		eventType, _ := raw["type"].(string)
		if eventType != "message" {
			continue
		}
		message, _ := raw["message"].(map[string]any)
		if message == nil {
			continue
		}
		role, _ := message["role"].(string)
		content, _ := message["content"].([]any)
		switch role {
		case "user":
			text := extractTextContent(content)
			if text == "" {
				continue
			}
			items = append(items, ConversationItem{Seq: nextSeq(), Kind: "user", Text: text})
		case "assistant":
			// Separamos thinking de text. Si hay thinking,
			// emitimos un item kind="thinking" primero y
			// después el assistant con sólo el text. Cada uno
			// recibe un seq distinto para que el upsert del
			// cliente V2 no los confunda.
			//
			// ponytail: si el assistant tiene SOLO thinking
			// sin text (edge case raro, normalmente pi emite
			// text después del thinking), descartamos ambos.
			// Mostrar un bloque thinking sin respuesta visible
			// confunde al usuario — el chat se ve "pensando
			// algo" pero no hay nada después.
			thinkingText, answerText := splitAssistantContent(content)
			if answerText != "" {
				if thinkingText != "" {
					items = append(items, ConversationItem{Seq: nextSeq(), Kind: "thinking", Text: thinkingText})
				}
				items = append(items, ConversationItem{Seq: nextSeq(), Kind: "assistant", Text: answerText})
			}
		case "toolResult":
			text := extractTextContent(content)
			toolName, _ := message["toolName"].(string)
			items = append(items, ConversationItem{
				Seq:      nextSeq(),
				Kind:     "tool_result",
				Text:     text,
				ToolName: strings.TrimSpace(toolName),
			})
		}
	}
	return items
}

// extractTextContent concatena todos los content blocks de tipo
// "text" del content array de un mensaje. Retorna el texto
// concatenado (los bloques se unen con newline para preservar
// la estructura).
func extractTextContent(content []any) string {
	var parts []string
	for _, c := range content {
		block, _ := c.(map[string]any)
		if block == nil {
			continue
		}
		btype, _ := block["type"].(string)
		if btype == "text" {
			text, _ := block["text"].(string)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// splitAssistantContent separa el content de un assistant en
// (thinking, answer). thinking viene de bloques tipo
// "thinking" y answer de bloques tipo "text". Si hay sólo
// thinking, retorna ("", "") — descartamos porque no tiene
// respuesta visible.
func splitAssistantContent(content []any) (thinking, answer string) {
	var thinkingParts, answerParts []string
	for _, c := range content {
		block, _ := c.(map[string]any)
		if block == nil {
			continue
		}
		btype, _ := block["type"].(string)
		switch btype {
		case "thinking":
			text, _ := block["thinking"].(string)
			if text != "" {
				thinkingParts = append(thinkingParts, text)
			}
		case "text":
			text, _ := block["text"].(string)
			if text != "" {
				answerParts = append(answerParts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(thinkingParts, "\n")),
		strings.TrimSpace(strings.Join(answerParts, "\n"))
}

// extractAssistantContentFromPayload parsea el payload crudo de
// un evento message_end (o cualquier evento con shape similar)
// y devuelve el contenido final del assistant como (text,
// thinking). Si el payload no tiene la estructura esperada,
// o si el role no es "assistant", retorna strings vacíos — el
// caller debe caer al draft acumulado.
//
// Devuelve también hasMessage para que el caller distinga entre
// "payload ausente" (caer al draft) y "payload presente pero
// no es assistant" (descartar el draft).
//
// ponytail: pi emite message_end también para mensajes con
// role=toolResult (el resultado de la tool se envía dos veces:
// una como tool_execution_end y otra como message_end con
// role=toolResult). Si extraemos el content de ese segundo
// evento, lo persistimos como assistant duplicando el tool
// result que ya capturamos por la otra vía. Por eso el check
// de role es OBLIGATORIO. Sin él, cada tool execution en el
// chat aparece dos veces (tool_result + assistant con el
// mismo texto).

// extractMessageUpdateDelta devuelve (type, deltaText) para un
// evento message_update del provider, tolerando DOS formatos:
//
//   1. Formato "plano" (versiones viejas de pi): el evento es
//      `{assistantMessageEvent: {type, delta}}` en event.Payload.
//   2. Formato "anidado" (pi actual visto en
//      agent-1784783206892149281-1): el evento tiene `payload`
//      adentro de event.Payload, y dentro está
//      `assistantMessageEvent: {type, delta}`. Equivale a
//      `{sessionId, type: "message_update", payload: {type:
//      "message_update", assistantMessageEvent: {type, delta}}}`.
//
// Si el payload no parece un message_update o le falta el
// assistantMessageEvent en ambas formas, devuelve ("", "") y el
// caller lo ignora. Esto reemplaza el Unmarshal top-level del
// código viejo que fallaba silenciosamente con el formato
// anidado, dejando los drafts sin actualizar (que era el
// bug oculto detrás de "perdí parte de la respuesta al
// refrescar").
func extractMessageUpdateDelta(payload json.RawMessage) (string, string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		AssistantMessageEvent struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		} `json:"assistantMessageEvent"`
	}
	// Try formato plano: assistantMessageEvent a nivel superior.
	if err := json.Unmarshal(payload, &p); err == nil &&
		(p.AssistantMessageEvent.Type != "" || p.AssistantMessageEvent.Delta != "") {
		return p.AssistantMessageEvent.Type, p.AssistantMessageEvent.Delta
	}
	// Fallback anidado: event.Payload.payload.assistantMessageEvent.
	var nested struct {
		Payload struct {
			AssistantMessageEvent struct {
				Type  string `json:"type"`
				Delta string `json:"delta"`
			} `json:"assistantMessageEvent"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(payload, &nested); err == nil &&
		(nested.Payload.AssistantMessageEvent.Type != "" || nested.Payload.AssistantMessageEvent.Delta != "") {
		return nested.Payload.AssistantMessageEvent.Type, nested.Payload.AssistantMessageEvent.Delta
	}
	return "", ""
}

func extractAssistantContentFromPayload(payload json.RawMessage) (text, thinking string, hasMessage bool, role string) {
	if len(payload) == 0 {
		return "", "", false, ""
	}
	var p struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", "", false, ""
	}
	if strings.TrimSpace(p.Message.Role) == "" && len(p.Message.Content) == 0 {
		// Payload presente pero sin estructura message válida
		// (ej. eventos viejos con shape distinto). El caller
		// debe caer al draft.
		return "", "", false, ""
	}
	// Solo extraer contenido de mensajes del assistant. Pi emite
	// message_end también para toolResult/user/etc — esos ya se
	// persisten por otras ramas del handler.
	if !strings.EqualFold(strings.TrimSpace(p.Message.Role), "assistant") {
		return "", "", true, p.Message.Role
	}
	var textParts, thinkingParts []string
	for _, c := range p.Message.Content {
		switch c.Type {
		case "text":
			if c.Text != "" {
				textParts = append(textParts, c.Text)
			}
		case "thinking":
			if c.Thinking != "" {
				thinkingParts = append(thinkingParts, c.Thinking)
			}
		}
	}
	return strings.TrimSpace(strings.Join(textParts, "\n")),
		strings.TrimSpace(strings.Join(thinkingParts, "\n")),
		true,
		p.Message.Role
}

func sanitizeSessionID(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = strings.ReplaceAll(safe, string(filepath.Separator), "_")
	return safe
}

func previewText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "\n\n…"
}

// previewTextLocal limpia el preview removiendo los bloques
// think inline del LLM. Es una copia local de
// assistantVisibleText del V2 — la lógica es chica y la
// necesitamos antes de pasar por goldmark. La razón de
// duplicar es no meter una dependencia entre el paquete
// application (server) y el paquete ui/v2 (cliente).
func previewTextLocal(text string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}
	// ponytail: usamos raw strings (backticks) para los strings
	// que contienen think tags. Go interpreta */ como cierre
	// de comentario de bloque incluso dentro de strings
	// literales "..." — eso rompe el parseo. Los raw strings
	// no tienen ese problema.
	const openTag = `<think>`
	const closeTag = `</think>`
	for {
		start := strings.Index(clean, openTag)
		if start < 0 {
			break
		}
		end := strings.Index(clean[start+len(openTag):], closeTag)
		if end < 0 {
			clean = clean[:start]
			break
		}
		end += start + len(openTag)
		clean = clean[:start] + clean[end+len(closeTag):]
	}
	return strings.TrimSpace(clean)
}

// cleanMarkdownForPreview limpia el markdown del texto del LLM
// antes de guardarlo en Session.LastPreview.
func cleanMarkdownForPreview(text string) string {
	clean := previewTextLocal(text)
	if clean == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := previewRenderer.Convert([]byte(clean), &buf); err != nil {
		return strings.TrimSpace(clean)
	}
	sanitized := previewPolicy.SanitizeBytes(buf.Bytes())
	stripped := htmlTagRE.ReplaceAllString(string(sanitized), "")
	stripped = html.UnescapeString(stripped)
	stripped = collapseWhitespaceRE.ReplaceAllString(stripped, " ")
	return strings.TrimSpace(stripped)
}

// Renderer y policy locales.
var (
	previewRenderer = goldmark.New(
		goldmark.WithExtensions(),
	)
	previewPolicy = bluemonday.NewPolicy()
)

func init() {
	previewPolicy.AllowElements(
		"p", "br", "hr",
		"strong", "em", "del", "s", "u",
		"code", "pre",
		"a", "ul", "ol", "li",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"blockquote",
		"table", "thead", "tbody", "tr", "th", "td",
	)
	previewPolicy.AllowAttrs("href").OnElements("a")
	previewPolicy.AllowURLSchemes("http", "https", "mailto")
	previewPolicy.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre")
}

var htmlTagRE = regexp.MustCompile(`<[^>]+>`)
// collapseWhitespaceRE colapsa CUALQUIER newline a espacio.
// ponytail: el preview del sidebar es una sola línea. Si
// dejamos un newline adentro, el sidebar lo renderiza como
// un salto de línea que rompe el layout (el CSS del preview
// tiene white-space:nowrap + ellipsis, pero el newline es
// un caracter whitespace igual). Goldmark típicamente pone
// un solo \n entre bloques (h1, p, etc), por eso usamos \n+
// en vez de \n{2,}.
var collapseWhitespaceRE = regexp.MustCompile(`\n+`)

func extractMessageText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return trimmed
}

func extractAssistantStopError(raw json.RawMessage) (string, bool) {
	var payload struct {
		Message struct {
			Role         string `json:"role"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", false
	}
	if strings.TrimSpace(payload.Message.Role) != "assistant" {
		return "", false
	}
	text := strings.TrimSpace(payload.Message.ErrorMessage)
	if text == "" {
		return "", false
	}
	if normalized := normalizeAssistantErrorText(text, payload.Message.StopReason); normalized != "" {
		return normalized, true
	}
	return text, true
}

func normalizeAssistantErrorText(text, stopReason string) string {
	combined := strings.ToLower(strings.TrimSpace(stopReason + " " + text))
	if strings.Contains(combined, "insufficient_credits") ||
		strings.Contains(combined, "payment required") ||
		strings.Contains(combined, "402") ||
		strings.Contains(combined, "créditos insuficientes") {
		return "Créditos insuficientes en el proveedor/modelo configurado."
	}
	return strings.TrimSpace(text)
}

// countByKind cuenta cuántos items tienen el kind dado.
// Usado por el auto-recovery para detectar transcripts con
// items duplicados por el mergeAssistantDelta bug.
func countByKind(items []ConversationItem, kind string) int {
	n := 0
	for _, it := range items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

// corruptionAlreadyHandled evita aplicar el trigger de
// divergencia por count si ya disparamos el de longitud de
// último assistant (en ese caso items ya apunta a piItems).
func corruptionAlreadyHandled(items, piItems []ConversationItem) bool {
	if len(items) != len(piItems) {
		return false
	}
	if len(items) == 0 {
		return false
	}
	// Quick check: si el primer item coincide, asumimos misma
	// fuente. No es 100% preciso pero suficiente para evitar
	// aplicar el segundo trigger redundante.
	return items[0].Seq == piItems[0].Seq && items[0].Kind == piItems[0].Kind && items[0].Text == piItems[0].Text
}
