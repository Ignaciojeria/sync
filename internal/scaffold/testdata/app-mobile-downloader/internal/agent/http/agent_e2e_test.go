package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
	agentmemory "lastmile-agents/internal/agent/infrastructure/memory"
	"lastmile-agents/internal/shared/server"
)

// newMemoryStoreForTest devuelve un SessionStore en memoria. Se usa
// en los tests E2E del agente para no escribir tmp/agent-sessions
// en el disco del developer.
func newMemoryStoreForTest() *agentmemory.SessionStore {
	return agentmemory.NewSessionStore()
}

// listSessionIDs devuelve los IDs de todas las sesiones del store.
// Sirve para limpiar el registry de renderers en t.Cleanup.
// Sirve para limpiar el registry de renderers en t.Cleanup.
func listSessionIDs(s *agentmemory.SessionStore) []string {
	sessions, _ := s.List(context.Background())
	out := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		id := strings.TrimSpace(sess.ID)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// fakeEventRunner es un Runner que emite eventos de pi simulados al
// canal de subscribe cuando el cliente hace Prompt. Lo usamos en
// los tests E2E del agente para verificar que el flujo prompt → SSE →
// HTML funciona end-to-end sin necesitar el binario `pi`.
//
// Limitaciones conocidas: este fake no implementa Abort real ni
// sesiones múltiples; es sólo lo mínimo para validar el happy path.
type fakeEventRunner struct {
	sessionID string
}

func (f *fakeEventRunner) Start(_ context.Context, spec agentapp.StartSpec) (agentapp.Runtime, error) {
	return &fakeRuntime{
		sessionID: spec.SessionID,
		events:    make(chan agentapp.Event, 32),
	}, nil
}

type fakeRuntime struct {
	sessionID string
	events    chan agentapp.Event
}

func (r *fakeRuntime) SessionID() string { return r.sessionID }

func (r *fakeRuntime) Prompt(ctx context.Context, msg string) error {
	// Materializamos el user_prompt en el transcript para que el
	// loadMessages lo vea. El Manager real hace esto vía
	// setSessionPreview; el fake lo simula mandando un evento
	// user_prompt que el handler ignora en su mayoría pero que el
	// journal acepta.
	go r.emitFakeTurn(msg)
	return nil
}

func (r *fakeRuntime) Steer(ctx context.Context, msg string) error { return nil }
func (r *fakeRuntime) Abort(ctx context.Context) error             { return nil }

func (r *fakeRuntime) Subscribe() (<-chan agentapp.Event, func()) {
	return r.events, func() {}
}

func (r *fakeRuntime) State() agentapp.RuntimeState {
	return agentapp.RuntimeState{Status: "running"}
}

func (r *fakeRuntime) Close() error { return nil }

// emitFakeTurn simula un turno de pi: message_start, dos
// text_delta, un bloque de thinking (thinking_delta) y una tool
// call con su resultado (tool_execution_end). Cada evento se
// materializa en transcript como en el runner real.
func (r *fakeRuntime) emitFakeTurn(_ string) {
	r.sessionID = strings.TrimSpace(r.sessionID)
	if r.sessionID == "" {
		return
	}

	// ponytail: respiramos entre eventos. Sin esto, el fake emite
	// todo antes de que el SSE handler alcance a hacer Subscribe,
	// y los eventos se pierden. En producción pi emite a un ritmo
	// natural humano (~50 ms por token) que le da tiempo al
	// cliente de engancharse. Acá simulamos eso.
	emit := func(evType string, payload []byte) {
		r.events <- agentapp.Event{
			SessionID: r.sessionID,
			Type:      evType,
			Payload:   payload,
		}
		time.Sleep(20 * time.Millisecond)
	}

	emit("message_start", json.RawMessage(`{"message":{"role":"assistant","content":[]}}`))
	// thinking arranca antes que el text (pi planifica primero)
	emit("message_update", json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":"el user quiere un pong"}}`))
	emit("message_update", json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":" — fácil"}}`))
	// text de respuesta
	emit("message_update", json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"Hola "}}`))
	emit("message_update", json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"V2"}}`))
	// tool call con su output
	emit("tool_execution_start", json.RawMessage(`{"toolCallId":"call_1","toolName":"bash","args":{"command":"echo hi"}}`))
	emit("tool_execution_end", json.RawMessage(`{"toolCallId":"call_1","toolName":"bash","result":{"content":[{"type":"text","text":"hi\n"}]}}`))
	emit("message_end", json.RawMessage(`{"message":{"role":"assistant","content":[{"type":"text","text":"Hola V2"}]}}`))
	emit("agent_end", json.RawMessage(`{"messages":[],"willRetry":false}`))
}

// TestE2E_HappyPath verifica el flujo:
//   1. Cliente hace POST /agent/sessions para crear
//   2. Cliente hace POST /agent/sessions/{id}/prompt
//   3. Cliente abre SSE /agent/sessions/{id}/events
//   4. Llega al menos un envelope kind=fragment con HTML del assistant
//   5. Llega un envelope kind=turn-end
func TestE2E_HappyPath(t *testing.T) {
	// Configurar sesión con renderer pre-registrado para que el
	// handler SSE emita HTML del shell actual (no fallback vacío).
	store := newMemoryStoreForTest()
	manager := agentapp.NewManager(store, &fakeEventRunner{})

	// Pre-registramos el renderer para la sesión que vamos a crear.
	// El page handler lo hace también cuando sirve la página, pero
	// acá vamos directo a POST sin pasar por GET /agent.
	t.Cleanup(func() {
		for _, s := range listSessionIDs(store) {
			agentapp.ClearSessionRenderer(s)
		}
	})

	srv := server.NewServer()
	noopEditor := func(h http.Handler) http.Handler { return h }
	// Tras el cutover 2026-07 toda la superficie del agente vive
	// en una sola Register (UI V2 + data handlers, sin flag ni
	// split V1/V2). Pasamos nil en sessionCostSvc porque este
	// test no ejerce el pre-render del budget bar.
	Register(srv, manager, nil, OIDCRefreshConfig{}, noopEditor, nil)

	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)

	// 1. POST /agent/sessions
	createBody := strings.NewReader(`{"title":"E2E","cwd":".","model":"default"}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create status = %d, want 200", createResp.StatusCode)
	}
	var created struct {
		Session agentapp.Session `json:"session"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	createResp.Body.Close()
	sessionID := strings.TrimSpace(created.Session.ID)
	if sessionID == "" {
		t.Fatalf("create: session id vacío")
	}

	// Pre-registramos el renderer para esta sesión (en producción
	// lo hace dashboardPageV2; acá lo simulamos).
	agentapp.SetSessionRenderer(sessionID, agentuiV2Renderer{})

	// 2. POST prompt
	promptBody := strings.NewReader(`{"message":"hola"}`)
	promptReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions/"+sessionID+"/prompt", promptBody)
	promptReq.Header.Set("Content-Type", "application/json")
	promptResp, err := http.DefaultClient.Do(promptReq)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if promptResp.StatusCode != http.StatusOK {
		t.Fatalf("prompt status = %d, want 200", promptResp.StatusCode)
	}
	promptResp.Body.Close()

	// 3. Abrimos SSE y leemos los primeros eventos
	sseReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/agent/sessions/"+sessionID+"/events?resume=live", nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	sseReq = sseReq.WithContext(sseCtx)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("sse connect: %v", err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status = %d, want 200", sseResp.StatusCode)
	}

	// 4-5. Leemos fragmentos del stream
	reader := bufio.NewReader(sseResp.Body)
	sawFragment := false
	sawThinking := false
	sawToolResult := false
	sawTurnEnd := false
	for !sawTurnEnd {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v (sawFragment=%v sawTurnEnd=%v)", err, sawFragment, sawTurnEnd)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var env struct {
				Kind     string `json:"kind"`
				ItemKind string `json:"itemKind"`
				HTML     string `json:"html"`
			}
			if err := json.Unmarshal([]byte(payload), &env); err != nil {
				continue
			}
			if env.Kind == "fragment" {
				switch env.ItemKind {
				case "assistant":
					sawFragment = true
					if !strings.Contains(env.HTML, "v2-item-assistant") {
						t.Errorf("fragment del assistant debería tener clase v2-item-assistant, got: %.200s", env.HTML)
					}
				case "thinking":
					sawThinking = true
					if !strings.Contains(env.HTML, "v2-item-thinking") {
						t.Errorf("fragment del thinking debería tener clase v2-item-thinking, got: %.200s", env.HTML)
					}
				case "tool_result":
					sawToolResult = true
					if !strings.Contains(env.HTML, "v2-item-tool_result") {
						t.Errorf("fragment del tool_result debería tener clase v2-item-tool_result, got: %.200s", env.HTML)
					}
				}
			}
			if env.Kind == "turn-end" {
				sawTurnEnd = true
			}
		}
	}
	if !sawFragment {
		t.Fatalf("SSE nunca emitió fragment del assistant")
	}
	if !sawThinking {
		t.Errorf("SSE nunca emitió fragment del thinking")
	}
	if !sawToolResult {
		t.Errorf("SSE nunca emitió fragment del tool_result")
	}
}

// TestE2E_AbortWorksWithoutPageVisit verifica que el POST
// /agent/sessions/{id}/abort funciona sin necesidad de pasar
// antes por GET /agent (la página). Tras el cutover, el wiring
// es único: la UI y los data handlers viven en la misma Register,
// así que abort no depende de ningún forwarder ni de un renderer
// per-session seteado por el page handler.
func TestE2E_AbortWorksWithoutPageVisit(t *testing.T) {
	store := newMemoryStoreForTest()
	manager := agentapp.NewManager(store, &fakeEventRunner{})

	srv := server.NewServer()
	noopEditor := func(h http.Handler) http.Handler { return h }
	Register(srv, manager, nil, OIDCRefreshConfig{}, noopEditor, nil)

	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)

	// Creamos sesión
	body := strings.NewReader(`{"title":"abort test","cwd":".","model":"default"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created struct {
		Session agentapp.Session `json:"session"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	sessionID := created.Session.ID

	// Abort. La respuesta es 200 OK porque el fake runtime lo acepta.
	abortReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions/"+sessionID+"/abort", strings.NewReader(`{}`))
	abortReq.Header.Set("Content-Type", "application/json")
	abortResp, err := http.DefaultClient.Do(abortReq)
	if err != nil {
		t.Fatalf("abort: %v", err)
	}
	if abortResp.StatusCode != http.StatusOK {
		t.Fatalf("abort status = %d, want 200", abortResp.StatusCode)
	}
	abortResp.Body.Close()
}

// TestE2E_RendererRegisteredOnCreateSession cubre el comportamiento
// del registry per-session: si un cliente crea una sesión via POST
// sin pasar antes por la página, los fragments SSE deben seguir
// emitiendo HTML V2 (clase v2-item-assistant). El test setea el
// renderer manualmente porque en este test no hay page handler que
// lo haga (vamos directo a POST); en producción lo hace
// dashboardPage antes de renderizar la página.
func TestE2E_RendererRegisteredOnCreateSession(t *testing.T) {
	store := newMemoryStoreForTest()
	manager := agentapp.NewManager(store, &fakeEventRunner{})

	srv := server.NewServer()
	noopEditor := func(h http.Handler) http.Handler { return h }
	Register(srv, manager, nil, OIDCRefreshConfig{}, noopEditor, nil)

	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)
	t.Cleanup(func() {
		for _, id := range listSessionIDs(store) {
			agentapp.ClearSessionRenderer(id)
		}
	})

	// POST create — NO visitamos GET /agent-v2 a propósito.
	// Este es el path donde el bug original se manifestaba.
	createBody := strings.NewReader(`{"title":"renderer-fix","cwd":".","model":""}`)
	createReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions", createBody)
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created struct {
		Session agentapp.Session `json:"session"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	createResp.Body.Close()
	sessionID := created.Session.ID

	// Pre-registramos manualmente el renderer V2 (en producción lo
	// hace dashboardPage cuando sirve la página; acá vamos directo
	// a POST sin pasar por GET /agent).
	agentapp.SetSessionRenderer(sessionID, agentuiV2Renderer{})

	// Verificamos que el registry tiene una entrada no-nil para el
	// sessionID y que el HTML que produce tiene la forma V2.
	r := agentapp.RendererFor(sessionID)
	if r == nil {
		t.Fatalf("renderer no quedó registrado tras el set, got nil")
	}
	// Además verificamos que el HTML que produce tiene la forma V2.
	html, err := r.RenderFragment(agentapp.ConversationItem{Kind: "assistant", Text: "x", Seq: 1})
	if err != nil {
		t.Fatalf("RenderFragment err = %v", err)
	}
	if !strings.Contains(html, "v2-item-assistant") {
		t.Fatalf("renderer registrado no es el V2, got html: %.400s", html)
	}

	// POST prompt.
	promptBody := strings.NewReader(`{"message":"hola"}`)
	promptReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions/"+sessionID+"/prompt", promptBody)
	promptReq.Header.Set("Content-Type", "application/json")
	promptResp, err := http.DefaultClient.Do(promptReq)
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}
	if promptResp.StatusCode != http.StatusOK {
		t.Fatalf("prompt status = %d", promptResp.StatusCode)
	}
	promptResp.Body.Close()

	// SSE.
	sseReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/agent/sessions/"+sessionID+"/events?resume=live", nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sseReq = sseReq.WithContext(sseCtx)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer sseResp.Body.Close()

	// Buscamos el primer fragment del assistant y verificamos que
	// su HTML tiene la clase V2 (v2-item-assistant) y NO la clase
	// V1 (chat-bubble / agent-glass).
	reader := bufio.NewReader(sseResp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var env struct {
			Kind     string `json:"kind"`
			ItemKind string `json:"itemKind"`
			HTML     string `json:"html"`
		}
		if err := json.Unmarshal([]byte(payload), &env); err != nil {
			continue
		}
		if env.Kind == "fragment" && env.ItemKind == "assistant" {
			if !strings.Contains(env.HTML, "v2-item-assistant") {
				t.Fatalf("fragment debería tener clase v2-item-assistant, got: %.400s", env.HTML)
			}
			if strings.Contains(env.HTML, "chat-bubble") || strings.Contains(env.HTML, "agent-glass") {
				t.Fatalf("fragment está usando HTML V1 en lugar de V2: %.400s", env.HTML)
			}
			// OK, regresamos sin esperar turn-end para no hacer el
			// test flaky con el fake runtime.
			return
		}
		if env.Kind == "turn-end" {
			t.Fatalf("turn-end llegó antes que ningún fragment del assistant")
		}
	}
}

// agentuiV2Renderer es un mirror del renderer V2 que vive en
// ui/v2. Como los tests del paquete http no pueden importar
// ui/v2 (import cycle), se duplica acá con la mínima superficie
// necesaria: un RenderFragment que produce data-msg y la clase
// v2-item-<kind>. El fix del renderer per-session asume que la
// firma coincide con agentapp.FragmentRenderer.
type agentuiV2Renderer struct{}

func (agentuiV2Renderer) RenderFragment(item agentapp.ConversationItem) (string, error) {
	// Versión simplificada para tests: data-msg + clases V2.
	var b bytes.Buffer
	b.WriteString(`<div data-msg="`)
	b.WriteString(fmt.Sprintf("%d", item.Seq))
	b.WriteString(`" data-kind="`)
	b.WriteString(item.Kind)
	b.WriteString(`" class="v2-item v2-item-`)
	b.WriteString(item.Kind)
	b.WriteString(`">`)
	b.WriteString(item.Text)
	b.WriteString(`</div>`)
	return b.String(), nil
}

// RenderToolResultPartial es un stub del test que NO se usa
// (los happy-path del chat no ejercen streaming de tool). Lo
// implementamos para satisfacer la interfaz FragmentRenderer.
func (agentuiV2Renderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	return "", nil
}
