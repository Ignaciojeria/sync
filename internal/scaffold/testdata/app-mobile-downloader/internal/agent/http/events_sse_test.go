package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// TestMain configura los directorios de transcripts y events a
// tempdirs separados durante la duración del test. Si usáramos el
// mismo dir para ambos, el archivo `.jsonl` se compartiría y el
// journal (que serializa con `"kind":"pi"` como provider name)
// contaminearía el read path del transcript (que deserializa a
// ConversationItem y leería `Kind:"pi"`). En producción los dirs
// son distintos (tmp/agent-transcripts vs tmp/agent-events), pero
// en este test bundle de package compartido lo hacemos explícito.
func TestMain(m *testing.M) {
	dirBase, err := os.MkdirTemp("", "agent-events-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "events test: mkdir tmp:", err)
		os.Exit(1)
	}
	transcriptsDir := filepath.Join(dirBase, "transcripts")
	eventsDir := filepath.Join(dirBase, "events")
	if err := os.MkdirAll(transcriptsDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "events test: mkdir transcripts:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(eventsDir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "events test: mkdir events:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dirBase)
	if err := os.Setenv("AGENT_EVENTS_DIR", eventsDir); err != nil {
		fmt.Fprintln(os.Stderr, "events test: setenv events dir:", err)
		os.Exit(1)
	}
	if err := os.Setenv("AGENT_TRANSCRIPTS_DIR", transcriptsDir); err != nil {
		fmt.Fprintln(os.Stderr, "events test: setenv transcripts dir:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// streamEventsServiceStub deja el manager mínimo necesario para
// ejercitar el SSE handler: Subscribe devuelve un canal que recibe
// events que el caller empuja desde el test.
type streamEventsServiceStub struct {
	ch     chan agentapp.Event
	cancel func()
}

// testFragmentRenderer es un FragmentRenderer mínimo para este
// test: emite HTML trivial con data-msg para que el cliente
// pueda hacer upsert. Es suficiente para ejercitar el flujo
// fragment → synthetic-end del SSE handler.
type testFragmentRenderer struct{}

func (testFragmentRenderer) RenderFragment(item agentapp.ConversationItem) (string, error) {
	if item.Text == "" {
		return "", nil
	}
	return `<div data-msg="` + fmt.Sprintf("%d", item.Seq) + `" data-kind="` + item.Kind + `">` + item.Text + `</div>`, nil
}

// RenderToolResultPartial es un stub del test que NO se usa
// (el test de SSE no ejercita streaming de tool). Lo
// implementamos para satisfacer la interfaz FragmentRenderer.
func (testFragmentRenderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	return "", nil
}

func (s *streamEventsServiceStub) List(context.Context) ([]agentapp.Session, error) {
	return nil, nil
}
func (s *streamEventsServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s *streamEventsServiceStub) Get(_ context.Context, id string) (agentapp.Session, error) {
	return agentapp.Session{ID: id}, nil
}
func (s *streamEventsServiceStub) Ensure(context.Context, string) error { return nil }
func (s *streamEventsServiceStub) Prompt(context.Context, string, string) error {
	return nil
}
func (s *streamEventsServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s *streamEventsServiceStub) Steer(context.Context, string, string) error  { return nil }
func (s *streamEventsServiceStub) Abort(context.Context, string) error          { return nil }
func (s *streamEventsServiceStub) Regenerate(context.Context, string) error    { return nil }
func (s *streamEventsServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return s.ch, s.cancel, nil
}
func (s *streamEventsServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s *streamEventsServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s *streamEventsServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s *streamEventsServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return agentapp.MergeResult{}, nil
}
func (s *streamEventsServiceStub) Delete(context.Context, string) error { return nil }
func (s *streamEventsServiceStub) Close() error                        { return nil }

// parseAgentFragmentEnvelopes lee la respuesta SSE y devuelve cada
// payload JSON de los eventos agent-fragment.
func parseAgentFragmentEnvelopes(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(strings.NewReader(body))
	var current map[string]any
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: agent-fragment"):
			current = nil
		case strings.HasPrefix(line, "data:"):
			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimSpace(payload)
			if payload == "" {
				continue
			}
			var env map[string]any
			if err := json.Unmarshal([]byte(payload), &env); err != nil {
				continue
			}
			current = env
		case line == "" && current != nil:
			out = append(out, current)
			current = nil
		}
	}
	return out
}

// TestStreamEvents_EmitsSyntheticTurnEndAfterMessageEnd verifica que
// tras un message_end materializado el server emite un envelope
// agent-fragment con kind="synthetic-end" — distinto del "turn-end"
// real. La distinción es importante porque pi emite UN message_end
// por cada chunk de streaming del assistant, no sólo al final del
// turno. Si synthetic-end y turn-end fueran el mismo envelope, el
// cliente haría flicker entre working/idle 4-5 veces por segundo
// durante un turno largo.
func TestStreamEvents_EmitsSyntheticTurnEndAfterMessageEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skip SSE end-to-end test in short mode")
	}
	stub := &streamEventsServiceStub{
		ch:     make(chan agentapp.Event, 16),
		cancel: func() {},
	}
	// ponytail: pre-registramos un renderer per-session para que
	// el text_delta del fake runner produzca un envelope
	// kind=fragment (sin renderer, streamOrSkip devuelve false y
	// el handler cae al envelope crudo, perdiendo el HTML que el
	// cliente espera). En producción lo hace dashboardPage antes
	// de servir la página; acá lo simulamos.
	agentapp.SetSessionRenderer("stream-events-A", testFragmentRenderer{})
	t.Cleanup(func() { agentapp.ClearSessionRenderer("stream-events-A") })

	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/stream-events-A/events?resume=live", nil)
	req.SetPathValue("id", "stream-events-A")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamEvents(stub)(rr, req)
	}()

	// Sincronizamos con la respuesta: el handler emite el envelope
	// "status" inicial antes de entrar al select principal. Una
	// vez visto, podemos empujar eventos con seguridad.
	waitForInitialStatus(t, rr)

	push := func(eventType, payload string) {
		env := agentapp.Event{Type: eventType, SessionID: "s1"}
		env.Payload = []byte(payload)
		stub.ch <- env
	}
	// Flujo realista: message_start abre el draft, los
	// message_update con text_delta acumulan texto y fijan el
	// Seq, message_end materializa el assistant item.
	push("message_start", `{}`)
	push("message_update", `{"assistantMessageEvent":{"type":"text_delta","delta":"¡Pong!"}}`)
	push("message_end", `{}`)

	// Esperamos el envelope synthetic-end. NO esperamos turn-end
	// real porque no lo emitimos en este test (no llega agent_end).
	waitForKind(t, rr, "synthetic-end", 1*time.Second)
	close(stub.ch)

	<-done

	body := rr.Body.String()
	envelopes := parseAgentFragmentEnvelopes(t, body)

	hasFragment := false
	hasSyntheticEnd := false
	for _, env := range envelopes {
		switch env["kind"] {
		case "fragment":
			hasFragment = true
		case "synthetic-end":
			hasSyntheticEnd = true
		}
	}

	if !hasFragment {
		t.Fatalf("expected at least one kind=fragment envelope from message_update/message_end, got:\n%s", body)
	}
	if !hasSyntheticEnd {
		t.Fatalf("expected synthetic kind=synthetic-end envelope after message_end materialization, got:\n%s", body)
	}
}

// TestStreamEvents_NoSyntheticTurnEndForToolExecution verifica que
// el turn-end sintético NO se dispara cuando llega un
// tool_execution_start (la sesión sigue activa y la herramienta
// todavía no terminó).
func TestStreamEvents_NoSyntheticTurnEndForToolExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skip SSE end-to-end test in short mode")
	}
	stub := &streamEventsServiceStub{
		ch:     make(chan agentapp.Event, 16),
		cancel: func() {},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/stream-events-B/events?resume=live", nil)
	req.SetPathValue("id", "stream-events-B")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamEvents(stub)(rr, req)
	}()

	time.Sleep(50 * time.Millisecond)
	env := agentapp.Event{Type: "tool_execution_start", SessionID: "s2"}
	env.Payload = []byte(`{"toolName":"bash","type":"bash","args":{}}`)
	stub.ch <- env

	time.Sleep(150 * time.Millisecond)
	close(stub.ch)
	<-done

	body := rr.Body.String()
	envelopes := parseAgentFragmentEnvelopes(t, body)

	for _, env := range envelopes {
		if env["kind"] == "turn-end" {
			t.Fatalf("tool_execution_start NO debería cerrar el turno; body:\n%s", body)
		}
	}
}

// waitForInitialStatus polls rr.Body until the SSE handler emits
// the initial status envelope (synchronized handshake). Polling
// a httptest.ResponseRecorder's body is safe because writes are
// flushed synchronously.
func waitForInitialStatus(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(rr.Body.String(), "event: status") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout esperando el envelope status inicial del SSE")
}

func waitForKind(t *testing.T, rr *httptest.ResponseRecorder, kind string, max time.Duration) {
	t.Helper()
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if strings.Contains(rr.Body.String(), `"kind":"`+kind+`"`) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout esperando envelope con kind=%q", kind)
}
