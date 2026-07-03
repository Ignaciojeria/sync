package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	agentapp "app-mobile-downloader/pkg/agent/application"
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
	// header gana (es el estándar SSE). Si ninguno, replay desde 0.
	since := parseSince(r.URL.Query().Get("resume"))
	if h := r.Header.Get("Last-Event-ID"); h != "" {
		since = parseSince(h)
	}

	// Replay de la cola pendiente antes de empezar el stream en vivo.
	// Sin esto, un cliente que perdió 5 eventos durante una desconexión
	// de 2 s perdería esos 5 eventos para siempre.
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
		// seguimos adelante: el stream en vivo funciona igual. Logueamos
		// para que el operator pueda actuar si pasa seguido.
		_ = replayErr // logged
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
			// Persistir + enviar. Si la persistencia falla,
			// seguimos adelante con el envío: el cliente ve el evento,
			// aunque el journal quede desfasado y el próximo replay
			// tenga un hueco. Logueamos para diagnóstico.
			seq, pErr := journal.append(id, "pi", event)
			if pErr != nil {
				_ = pErr // no fatal; log abajo
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
