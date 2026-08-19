package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/server"
)

func eventsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/sessions/{id}/events", requireEditor(streamEvents(manager)))
}

func streamEvents(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		ch, cancel, err := manager.Subscribe(r.Context(), id)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		defer cancel()

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: "streaming not supported"})
			return
		}

		journal, err := openEventsJournal()
		if err != nil {
			writeError(w, server.HTTPError{Status: http.StatusServiceUnavailable, Detail: "events journal unavailable"})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		resumeRaw := strings.TrimSpace(r.URL.Query().Get("resume"))
		since := parseSince(resumeRaw)
		liveOnly := resumeRaw == "live"
		if h := r.Header.Get("Last-Event-ID"); h != "" {
			since = parseSince(h)
			liveOnly = false
		}

		if !liveOnly && since > 0 {
			entries, replayErr := journal.replay(id, since, nil)
			if replayErr == nil {
				for _, e := range entries {
					if err := writeSSERaw(w, e.Kind, e.Seq, e.Payload); err != nil {
						return
					}
				}
				if len(entries) > 0 {
					flusher.Flush()
				}
			}
		}

		if err := writeSSE(w, "status", map[string]any{"status": "connected", "sessionId": id}); err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
			return
		}
		flusher.Flush()

		keepalive := time.NewTicker(10 * time.Second)
		defer keepalive.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				seq, appended, pErr := journal.appendOnce(id, "pi", event)
				if pErr == nil && appended {
					if seqSetter, ok := manager.(interface {
						SetLastSeq(context.Context, string, uint64)
					}); ok {
						seqSetter.SetLastSeq(r.Context(), id, seq)
					}
					agentapp.MaterializeEvent(id, seq, event)
				}
				// Emitimos siempre el envelope asimilado. Si el
				// renderer concreto está disponible, el cliente
				// recibe HTML listo para insertar; si no, recibe
				// una señal de fase. El provider crudo se queda
				// en el journal para replay/debug, no se reenvía.
				if fragment, emit := agentapp.RenderFragment(id, seq, event); emit {
					payload, mErr := json.Marshal(fragment)
					if mErr != nil {
						continue
					}
					if err := writeSSERaw(w, "agent-fragment", seq, payload); err != nil {
						return
					}
					flusher.Flush()
					// ponytail: el `continue` que estaba acá
					// rompía la experiencia de chat durante
					// tool calls. RenderFragment devuelve
					// `{kind:"phase", phase:"tooling"}` para
					// tool_execution_start — un envelope SIN
					// HTML. Saltarnos EmitFragment significaba
					// que el tool card con args (recién
					// materializado al transcript por
					// MaterializeEvent) NUNCA llegaba al
					// cliente durante streaming: el usuario
					// veía el badge cambiar a "tooling" pero
					// el feed NO crecía, así que el sticky
					// scroll no tenía nada nuevo a qué
					// seguir. El tool card aparecía sólo
					// después de un page reload (donde
					// LoadConversationHistory lo recuperaba
					// del transcript).
					//
					// Lógica nueva: sólo skipeamos EmitFragment
					// cuando RenderFragment ya emitió HTML
					// (kind="fragment"). Para envelopes
					// auxiliares (phase, turn-end, queue)
					// caemos al EmitFragment de abajo, que
					// busca el item recién materializado por
					// seq y emite su HTML si existe. El
					// cliente termina recibiendo DOS envelopes
					// por evento (phase + fragment), lo cual
					// está bien — su state machine de phase es
					// idempotente y el upsert por data-msg hace
					// que el segundo envelope reemplace el
					// mismo nodo, no se duplica visualmente.
					if fragment.Kind == "fragment" {
						continue
					}
				}
				// Materialización terminal (message_end,
				// tool_execution_start, runtime_error): empujamos
				// el item recién escrito al cliente como HTML
				// final. EmitFragment lee el último seq del
				// transcript y decide si hay algo nuevo que
				// pintar.
				if fe, emit := agentapp.EmitFragment(id, seq); emit {
					payload, mErr := json.Marshal(fe)
					if mErr != nil {
						continue
					}
					if err := writeSSERaw(w, "agent-fragment", seq, payload); err != nil {
						return
					}
					flusher.Flush()
				}
				// ponytail: tras materializar un message_end, el turno
				// del usuario está realmente cerrado (el assistant ya
				// terminó su mensaje — con o sin texto visible). El
				// runtime a veces NO emite turn_end explícito después
				// de un prompt normal — sólo lo hace en agent_end o
				// runtime_exit cuando el proceso entero termina.
				// Sin esta señal sintética, el state machine del
				// cliente quedaba atascado en turnPhase="answering"
				// y el banner "Working…" seguía visible. Si después
				// llega un turn_end real del runtime, es idempotente:
				// el handler limpia los mismos flags otra vez.
				//
				// Va afuera del bloque EmitFragment porque cuando el
				// assistant emite un message_end sin texto (draft
				// vacío), EmitFragment no tiene item que renderizar
				// y devuelve false — el cliente igual necesita la
				// señal para limpiar su state machine.
				if event.Type == "message_end" {
					// ponytail: synthetic-end se distingue del turn-end
					// real. Pi emite UN message_end por cada chunk de
					// streaming del assistant (no sólo al final del
					// turno). Si cada message_end generara un
					// turn-end, el state machine del cliente bailaría
					// entre "thinking" y "idle" 4-5 veces por segundo
					// durante un turno largo — eso es el flicker que
					// el user reportó. La señal sintética sólo
					// destraba el composer (porque el draft ya está
					// cerrado); el turn-end real (agent_end,
					// turn_end, runtime_exit) es el que limpia el
					// phase a idle y vacía el badge. La distinción se
					// hace con el kind del envelope.
					endPayload, endErr := json.Marshal(agentapp.FragmentEvent{Kind: "synthetic-end"})
					if endErr == nil {
						if err := writeSSERaw(w, "agent-fragment", seq, endPayload); err != nil {
							return
						}
						flusher.Flush()
					}
				}
			case <-keepalive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, eventName string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, payload)
	return err
}

func writeSSERaw(w io.Writer, eventName string, seq uint64, payload []byte) error {
	_, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", seq, eventName, payload)
	return err
}

type eventsJournal struct {
	dir  string
	mu   sync.Mutex
	seen map[string]map[string]uint64
}

var (
	journalOpenMu sync.Mutex
	journalGlobal *eventsJournal
)

func openEventsJournal() (*eventsJournal, error) {
	dir := strings.TrimSpace(os.Getenv("AGENT_EVENTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-events"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("mkdir events dir %q: %w", dir, err)
	}
	journalOpenMu.Lock()
	defer journalOpenMu.Unlock()
	if journalGlobal != nil && journalGlobal.dir == dir {
		return journalGlobal, nil
	}
	journalGlobal = &eventsJournal{dir: dir, seen: map[string]map[string]uint64{}}
	return journalGlobal, nil
}

type journalEntry struct {
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Time    string          `json:"time"`
}

func (j *eventsJournal) path(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = strings.ReplaceAll(safe, string(filepath.Separator), "_")
	return filepath.Join(j.dir, safe+".jsonl")
}

func (j *eventsJournal) lastSeq(sessionID string) (uint64, error) {
	f, err := os.Open(j.path(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	var max uint64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e journalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil && e.Seq > max {
			max = e.Seq
		}
	}
	if err := sc.Err(); err != nil {
		return max, err
	}
	return max, nil
}

func (j *eventsJournal) replay(sessionID string, since uint64, dst []journalEntry) ([]journalEntry, error) {
	f, err := os.Open(j.path(sessionID))
	if errors.Is(err, os.ErrNotExist) {
		return dst, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e journalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		if e.Seq > since {
			dst = append(dst, e)
		}
	}
	if err := sc.Err(); err != nil {
		return dst, err
	}
	return dst, nil
}

func (j *eventsJournal) appendOnce(sessionID, kind string, payload any) (uint64, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, false, fmt.Errorf("marshal payload: %w", err)
	}
	sig := kind + "\x00" + string(body)
	if seq, ok := j.seenSeq(sessionID, sig); ok {
		return seq, false, nil
	}

	seq, err := j.lastSeq(sessionID)
	if err != nil {
		return 0, false, fmt.Errorf("read last seq: %w", err)
	}
	seq++
	enc, err := json.Marshal(journalEntry{Seq: seq, Kind: kind, Payload: body, Time: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return seq, false, fmt.Errorf("marshal entry: %w", err)
	}
	f, err := os.OpenFile(j.path(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return seq, false, fmt.Errorf("open %s: %w", j.path(sessionID), err)
	}
	defer f.Close()
	if _, err := f.Write(append(enc, '\n')); err != nil {
		return seq, false, fmt.Errorf("write: %w", err)
	}
	j.rememberSeq(sessionID, sig, seq)
	return seq, true, nil
}

func (j *eventsJournal) seenSeq(sessionID, sig string) (uint64, bool) {
	if j.seen == nil {
		return 0, false
	}
	seq, ok := j.seen[sessionID][sig]
	return seq, ok
}

func (j *eventsJournal) rememberSeq(sessionID, sig string, seq uint64) {
	if j.seen == nil {
		j.seen = map[string]map[string]uint64{}
	}
	if j.seen[sessionID] == nil {
		j.seen[sessionID] = map[string]uint64{}
	}
	if len(j.seen[sessionID]) >= 512 {
		j.seen[sessionID] = map[string]uint64{}
	}
	j.seen[sessionID][sig] = seq
}

func parseSince(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// ponytail: journalPersistUserPrompt persiste un evento
// user_prompt directamente al journal y al runtimeEventsStore
// (DB). Lo usa el POST handler para que el user_prompt quede
// disponible para el SSE handler que se conecte después. El
// rebuild del journal siempre lo va a leer del archivo, no
// necesitamos una segunda copia en DB.
//
// Antes el POST handler hacía runtimeEventsStore.Append directo
// (best-effort, error swallowed). Eso causaba que el user_prompt
// NO quedara en el journal consistente. Cuando el user refrescaba
// la página, el rebuild del journal no encontraba el user_prompt
// y el feed se renderizaba sin la pregunta del user (sólo la
// respuesta del agent).
//
// Ahora journalPersistUserPrompt usa journal.appendOnce que es
// idempotente (seen-by-sig). Si el SSE handler está activo y
// recibe el evento via broadcast, no hay duplicado.
func journalPersistUserPrompt(ctx context.Context, sessionID, text string) error {
	journal, err := openEventsJournal()
	if err != nil {
		return err
	}
	payload := agentapp.Event{
		SessionID: sessionID,
		Type:      "user_prompt",
		Payload:   []byte(`{"text":` + strconv.Quote(strings.TrimSpace(text)) + `}`),
	}
	_, _, err = journal.appendOnce(sessionID, "pi", payload)
	return err
}
