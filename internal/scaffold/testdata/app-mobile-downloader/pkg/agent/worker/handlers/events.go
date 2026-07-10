package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	agentapp "scaffoldxd1/pkg/agent/application"
)

// handleEvents sirve el stream SSE para una sesión del agente.
//
// El handler dispatcheado en sessions.go invoca esta función cuando el
// path termina en /events y el método es GET. Implementamos SSE con
// stdlib: http.Flusher para push inmediato y un ticker de keepalive
// para vencer el timeout del proxy inverso (~30 s en exe.dev).
//
// Resilencia extra (Tier 2 del plan de robustez):
//   - Cada evento lleva `id: <seq>` para que un cliente que se
//     desconecta unos segundos pueda enviar `Last-Event-ID` al
//     reconectarse y recibir la cola de eventos que se perdió.
//   - Persistimos cada evento en $AGENT_EVENTS_DIR/<sid>.jsonl (default
//     tmp/agent-events) para que el replay funcione aunque el worker
//     reinicie entre la desconexión y la reconexión.
func handleEvents(w http.ResponseWriter, r *http.Request, mgr agentapp.AgentService, id string) {
	ch, cancel, err := mgr.Subscribe(r.Context(), id)
	if err != nil {
		status, detail := mapSessionError(err)
		http.Error(w, detail, status)
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	journal, err := openEventsJournal()
	if err != nil {
		// Fallar fuerte: si no podemos abrir el directorio del journal,
		// tampoco podemos garantizar el replay. Mejor 503 que un stream
		// que el cliente cree durable pero que perderemos si se cae.
		http.Error(w, "events journal unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Resume: ?resume=N o header Last-Event-ID. Si ambos presentes, el
	// header gana (es el estándar SSE). `resume=live` NO debe tocar el
	// journal histórico: la UI abre la sesión en modo shell-first y el
	// stream queda sólo para eventos nuevos.
	resumeRaw := r.URL.Query().Get("resume")
	since := parseSince(resumeRaw)
	liveOnly := resumeRaw == "live"
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		since = parseSince(h)
		liveOnly = false
	}

	// Replay solo cuando el cliente trae un cursor real. En modo live
	// puro no escaneamos el journal: conectar a una sesión vieja debe
	// ser O(1), no O(tamaño del archivo).
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
		} else {
			// Si falla el replay (no es crítico, sólo perdemos replay),
			// seguimos adelante: el stream en vivo funciona igual.
			_ = replayErr
		}
	}

	if err := writeSSE(w, "status", map[string]any{"status": "connected", "sessionId": id}); err != nil {
		return
	}
	flusher.Flush()

	// Keepalive de 10s: el proxy inverso de exe.dev corta conexiones
	// idle en torno a 30 s. Con 10 s de keepalive, la conexión siempre
	// lleva tráfico y el navegador no la recicla.
	keepalive := time.NewTicker(10 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			// Persistir una sola vez por evento aunque existan múltiples
			// conexiones SSE mirando la misma sesión (tab duplicada,
			// reconnect superpuesto, etc.). Sin esto, cada subscriber
			// reappendía el mismo evento y duplicaba transcript / replay.
			seq, appended, pErr := journal.appendOnce(id, "pi", event)
			if pErr != nil {
				_ = pErr // no fatal; log abajo
			} else if appended {
				if seqSetter, ok := mgr.(interface {
					SetLastSeq(context.Context, string, uint64)
				}); ok {
					seqSetter.SetLastSeq(r.Context(), id, seq)
				}
				agentapp.MaterializeEvent(id, seq, event)
			}
			payload, mErr := json.Marshal(event)
			if mErr != nil {
				continue
			}
			if err := writeSSERaw(w, "pi", seq, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepalive.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// writeSSE serializa value como JSON y lo manda como mensaje SSE
// SIN id (usado para el evento de status inicial, que no entra al
// journal).
func writeSSE(w http.ResponseWriter, eventName string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, payload)
	return err
}

// writeSSERaw manda un mensaje SSE con id explícito. El payload ya
// viene en JSON (formato `{"seq":N,"kind":"...","payload":<json>}`).
func writeSSERaw(w io.Writer, eventName string, seq uint64, payload []byte) error {
	_, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", seq, eventName, payload)
	return err
}
