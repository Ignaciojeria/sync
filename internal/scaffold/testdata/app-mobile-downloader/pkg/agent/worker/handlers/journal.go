package handlers

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// eventsJournal es un log append-only por sesión. Cada evento que el
// worker emite hacia el SSE también queda persistido acá para que un
// cliente que se desconectó unos segundos pueda reanudar desde el
// último ID recibido sin perder mensajes.
//
// Diseño:
//   - Un archivo jsonl por sesión en $AGENT_EVENTS_DIR/<session>.jsonl
//     (default tmp/agent-events).
//   - Cada línea es un objeto JSON {"seq":N, "kind":"pi"|"status",
//     "payload":<json>, "time":"<RFC3339Nano>"}. El handle se abre/cierra
//     en cada escritura — barato y robusto contra cierres abruptos del
//     worker (sólo perdés los últimos bytes no flushed del kernel; nada
//     queda en memoria sin fsync).
//   - Para evitar races entre dos handlers /events concurrentes sobre
//     la misma sesión, el append toma un mutex por proceso. El worker
//     normalmente tiene un único cliente SSE por sesión, así que el
//     lock no se contiende en la práctica.
//   - El replay lee el archivo en orden ascendente de seq y emite todo
//     lo que tenga seq > since. seq=0 reproduce desde el principio.
//
// Tradeoff conocido: no usa Postgres/CRDT. Es un simple file journal
// para que un cliente (browser tab) pueda retomar tras desconexión.
// Si en el futuro necesitamos sync multi-cliente (varias tabs viendo
// la misma conversación), acá entra la opción de migrar a ElectricSQL
// o similar. Hasta allora esto cubre el 99% de los casos.
type eventsJournal struct {
	dir string
	mu  sync.Mutex
}

func openEventsJournal() (*eventsJournal, error) {
	dir := strings.TrimSpace(os.Getenv("AGENT_EVENTS_DIR"))
	if dir == "" {
		dir = "tmp/agent-events"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("mkdir events dir %q: %w", dir, err)
	}
	return &eventsJournal{dir: dir}, nil
}

type journalEntry struct {
	Seq     uint64          `json:"seq"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
	Time    string          `json:"time"`
}

// path devuelve la ruta del jsonl para una sesión. sessionID viene del
// URL (es un id opaco que generamos nosotros), pero por defensa le
// escapamos cualquier "/" para no permitir escape de directorio.
func (j *eventsJournal) path(sessionID string) string {
	safe := strings.ReplaceAll(sessionID, "/", "_")
	safe = strings.ReplaceAll(safe, "..", "_")
	safe = strings.ReplaceAll(safe, string(filepath.Separator), "_")
	return filepath.Join(j.dir, safe+".jsonl")
}

// lastSeq devuelve el mayor seq ya emitido para esta sesión, o 0 si
// no hay journal todavía.
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
	// Algunos eventos pi tienen payloads grandes (tool args, etc).
	// Subimos el buffer para no truncar.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e journalEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err == nil {
			if e.Seq > max {
				max = e.Seq
			}
		}
	}
	if err := sc.Err(); err != nil {
		return max, err
	}
	return max, nil
}

// replay devuelve todas las entradas con seq > since, ordenadas por
// seq ascendente. dst es un slice reutilizable (se appendea).
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
			// línea corrupta: la saltamos. Un día podríamos
			// loggearla, pero escribir a log por cada línea
			// ralentiza replay innecesariamente.
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

// append bloquea hasta tener un lock (proceso-local) y luego escribe
// atómicamente una línea al jsonl. Devuelve el seq asignado.
func (j *eventsJournal) append(sessionID, kind string, payload any) (uint64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	seq, err := j.lastSeq(sessionID)
	if err != nil {
		return 0, fmt.Errorf("read last seq: %w", err)
	}
	seq++

	body, err := json.Marshal(payload)
	if err != nil {
		return seq, fmt.Errorf("marshal payload: %w", err)
	}
	enc, err := json.Marshal(journalEntry{
		Seq:     seq,
		Kind:    kind,
		Payload: body,
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return seq, fmt.Errorf("marshal entry: %w", err)
	}

	f, err := os.OpenFile(j.path(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return seq, fmt.Errorf("open %s: %w", j.path(sessionID), err)
	}
	defer f.Close()
	if _, err := f.Write(append(enc, '\n')); err != nil {
		return seq, fmt.Errorf("write: %w", err)
	}
	return seq, nil
}

// parseSince acepta lo que venga en el header `Last-Event-ID` o el
// query param `resume`, y lo convierte a uint64. Cualquier input
// inválido (string vacío, "NaN", "abc") resulta en 0 = "replay desde
// el principio".
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
