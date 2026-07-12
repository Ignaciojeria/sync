package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

type runtimeEventsStoreContract interface {
	Append(ctx context.Context, sessionID string, kind string, payload any) (uint64, error)
	ListAfter(ctx context.Context, sessionID string, after uint64, limit int) ([]agentapp.RuntimeEventRecord, error)
}

var runtimeEventsStore runtimeEventsStoreContract

func SetRuntimeEventsStore(store runtimeEventsStoreContract) {
	runtimeEventsStore = store
}

func eventsHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	fuego.Get(s.Server, "/agent/sessions/{id}/events", streamEvents(manager), fuego.OptionMiddleware(requireEditor))
}

func streamEvents(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		ch, cancel, err := manager.Subscribe(c.Context(), id)
		if err != nil {
			return nil, mapSessionError(err)
		}
		defer cancel()

		flusher, ok := c.Response().(http.Flusher)
		if !ok {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "streaming not supported"}
		}

		journal, err := openEventsJournal()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusServiceUnavailable, Detail: "events journal unavailable"}
		}

		w := c.Response()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		resumeRaw := c.QueryParam("resume")
		since := parseSince(resumeRaw)
		liveOnly := resumeRaw == "live"
		if h := c.Request().Header.Get("Last-Event-ID"); h != "" {
			since = parseSince(h)
			liveOnly = false
		}

		if !liveOnly && since > 0 {
			replayed := false
			if runtimeEventsStore != nil {
				rows, replayErr := runtimeEventsStore.ListAfter(c.Context(), id, since, 500)
				if replayErr == nil {
					for _, row := range rows {
						if err := writeSSERaw(w, row.Kind, row.Offset, row.Payload); err != nil {
							return nil, nil
						}
					}
					if len(rows) > 0 {
						flusher.Flush()
					}
					replayed = true
				}
			}
			if !replayed {
				entries, replayErr := journal.replay(id, since, nil)
				if replayErr == nil {
					for _, e := range entries {
						if err := writeSSERaw(w, e.Kind, e.Seq, e.Payload); err != nil {
							return nil, nil
						}
					}
					if len(entries) > 0 {
						flusher.Flush()
					}
				}
			}
		}

		if err := writeSSE(w, "status", map[string]any{"status": "connected", "sessionId": id}); err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}
		flusher.Flush()

		keepalive := time.NewTicker(10 * time.Second)
		defer keepalive.Stop()

		ctx := c.Context()
		for {
			select {
			case <-ctx.Done():
				return nil, nil
			case event, ok := <-ch:
				if !ok {
					return nil, nil
				}
				seq, appended, pErr := journal.appendOnce(id, "pi", event)
				if pErr == nil && appended {
					if runtimeEventsStore != nil {
						if dbOffset, err := runtimeEventsStore.Append(c.Context(), id, "pi", event); err != nil {
							slog.Warn("agent runtime events append failed", "session_id", id, "err", err)
						} else if dbOffset > 0 {
							seq = dbOffset
						}
					}
					if seqSetter, ok := manager.(interface {
						SetLastSeq(context.Context, string, uint64)
					}); ok {
						seqSetter.SetLastSeq(c.Context(), id, seq)
					}
					agentapp.MaterializeEvent(id, seq, event)
				}
				payload, mErr := json.Marshal(event)
				if mErr != nil {
					continue
				}
				if err := writeSSERaw(w, "pi", seq, payload); err != nil {
					return nil, nil
				}
				flusher.Flush()
			case <-keepalive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					return nil, nil
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

var _ = agentapp.Event{}
