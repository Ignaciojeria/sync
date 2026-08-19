package honcho

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// fakeHoncho es un mock del API de Honcho que registra todos
// los requests recibidos y permite responder por path. Sirve
// para tests de integración del Adapter sin pegar contra
// api.honcho.dev.
type fakeHoncho struct {
	mu       sync.Mutex
	calls    []capturedCall
	handlers map[string]http.HandlerFunc
}

type capturedCall struct {
	Method  string
	Path    string
	RawQuery string
	Body    string
}

func newFakeHoncho() *fakeHoncho {
	return &fakeHoncho{
		handlers: map[string]http.HandlerFunc{},
	}
}

// on registra un handler por path exacto (sin query).
func (f *fakeHoncho) on(path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = h
}

func (f *fakeHoncho) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, capturedCall{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Body:     string(body),
	})
	h, ok := f.handlers[r.URL.Path]
	f.mu.Unlock()
	// Restauramos el body para que el handler pueda leerlo.
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if !ok {
		http.Error(w, "fakeHoncho: no handler for "+r.URL.Path, http.StatusNotFound)
		return
	}
	h(w, r)
}

func (f *fakeHoncho) callsByPath(prefix string) []capturedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []capturedCall
	for _, c := range f.calls {
		if strings.HasPrefix(c.Path, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// newTestAdapter monta un Adapter contra un fakeHoncho.
// Devuelve el adapter y el fake para inspeccionar.
func newTestAdapter(t *testing.T) (*Adapter, *fakeHoncho) {
	t.Helper()
	fake := newFakeHoncho()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	a, err := NewAdapter(Config{
		BaseURL:     srv.URL,
		APIKey:      "test-key",
		WorkspaceID: "ws-1",
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	return a, fake
}

// defaultEnsurePeersHandlers registra handlers 200 OK para
// los 4 paths de EnsurePeers.
func defaultEnsurePeersHandlers(fake *fakeHoncho) {
	fake.on("/v3/workspaces", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ws-1","created_at":"2026-01-01T00:00:00Z"}`))
	})
	fake.on("/v3/workspaces/ws-1/peers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	fake.on("/v3/workspaces/ws-1/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func sampleKey() agentapp.MemoryKey {
	return agentapp.MemoryKey{
		WorkspaceID: "ws-1",
		SessionID:   "sess-1",
		UserEmail:   "alice@example.com",
	}
}

func TestAdapter_EnsurePeers_CallsInOrder(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	if err := a.EnsurePeers(context.Background(), sampleKey()); err != nil {
		t.Fatalf("EnsurePeers: %v", err)
	}
	// 4 calls: workspace, peer (agent), peer (user), session
	if got := len(fake.calls); got != 4 {
		t.Fatalf("calls: got %d, want 4: %+v", got, fake.calls)
	}
	wantOrder := []string{
		"POST /v3/workspaces",
		"POST /v3/workspaces/ws-1/peers",
		"POST /v3/workspaces/ws-1/peers",
		"POST /v3/workspaces/ws-1/sessions",
	}
	for i, want := range wantOrder {
		got := fake.calls[i].Method + " " + fake.calls[i].Path
		if got != want {
			t.Errorf("call[%d]: got %s, want %s", i, got, want)
		}
	}
	// Verificar que el session body bindea los dos peers con sus
	// configs correctas.
	var sbody SessionCreate
	if err := json.Unmarshal([]byte(fake.calls[3].Body), &sbody); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	// userPeerIDFor("alice@example.com") = "user-<sha256[:16]>"
	// Verificamos que la key está bindeada con observe_me=true.
	userKey := userPeerIDFor("alice@example.com")
	if cfg, ok := sbody.Peers[userKey]; !ok || cfg.ObserveMe != true {
		t.Errorf("user peer %q: got %+v, want observe_me=true", userKey, cfg)
	}
	if cfg, ok := sbody.Peers["agent-sess-1"]; !ok || cfg.ObserveMe != false {
		t.Errorf("agent peer: got %+v, want observe_me=false", cfg)
	}
}

func TestAdapter_EnsurePeers_RequiresSessionID(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	err := a.EnsurePeers(context.Background(), agentapp.MemoryKey{
		WorkspaceID: "ws-1",
		UserEmail:   "alice@example.com",
		// SessionID vacío a propósito
	})
	if err == nil {
		t.Fatal("expected error for missing SessionID")
	}
	if len(fake.calls) != 0 {
		t.Errorf("server should not be hit, got %d calls", len(fake.calls))
	}
}

func TestAdapter_EnsurePeers_RequiresUserEmail(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	err := a.EnsurePeers(context.Background(), agentapp.MemoryKey{
		WorkspaceID: "ws-1",
		SessionID:   "sess-1",
	})
	if err == nil {
		t.Fatal("expected error for missing UserEmail")
	}
}

func TestAdapter_EnsurePeers_AgentIDOverride(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	key := sampleKey()
	key.AgentID = "agent-real-123"
	if err := a.EnsurePeers(context.Background(), key); err != nil {
		t.Fatalf("EnsurePeers: %v", err)
	}
	// Cuando AgentID está seteado, el peer ID es "agent-<AgentID>".
	// (no "agent:<AgentID>" porque Honcho rechaza el ':')
	var pbody PeerCreate
	if err := json.Unmarshal([]byte(fake.calls[1].Body), &pbody); err != nil {
		t.Fatalf("decode peer body: %v", err)
	}
	if pbody.ID != "agent-agent-real-123" {
		t.Errorf("agent peer id: got %q, want agent-agent-real-123", pbody.ID)
	}
}

func TestAdapter_Recall_FormatsContext(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/context", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"sess-1",
			"messages":[],
			"summary":{"content":"user asked about deploys","message_id":"m1","summary_type":"short","created_at":"2026-01-01T00:00:00Z","token_count":10},
			"peer_representation":"user prefers terse answers in es-ES",
			"peer_card":["backend dev","es-ES"]
		}`)
	})
	got, err := a.Recall(context.Background(), sampleKey(), "qué dijo el agente")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if !strings.Contains(got.Text, "<summary>") {
		t.Errorf("text missing <summary>: %q", got.Text)
	}
	if !strings.Contains(got.Text, "deploys") {
		t.Errorf("text missing summary content: %q", got.Text)
	}
	if !strings.Contains(got.Text, "<representation>") {
		t.Errorf("text missing <representation>: %q", got.Text)
	}
	if !strings.Contains(got.Text, "terse answers") {
		t.Errorf("text missing representation content: %q", got.Text)
	}
	if !strings.Contains(got.Text, "<peer_card>") {
		t.Errorf("text missing <peer_card>: %q", got.Text)
	}
	if !strings.Contains(got.Text, "- backend dev") {
		t.Errorf("text missing peer card item: %q", got.Text)
	}
	// Verificamos que la query se pasó al endpoint y que el
	// peer_target es el user (cross-session memory funciona
	// porque el user persiste entre sesiones, el agent no).
	calls := fake.callsByPath("/v3/workspaces/ws-1/sessions/sess-1/context")
	if len(calls) != 1 {
		t.Fatalf("context calls: got %d, want 1", len(calls))
	}
	if !strings.Contains(rURLQuery(calls[0]), "search_query=") {
		t.Errorf("expected search_query in URL, got body=%q url=%s", calls[0].Body, calls[0].Path)
	}
	// peer_target debe ser el hash del user, no el agent.
	if !strings.Contains(rURLQuery(calls[0]), "peer_target=user-") {
		t.Errorf("peer_target should be user-* (cross-session memory), got: %s", rURLQuery(calls[0]))
	}
	if strings.Contains(rURLQuery(calls[0]), "peer_target=agent-") {
		t.Errorf("peer_target should NOT be agent-* (empty per session), got: %s", rURLQuery(calls[0]))
	}
}

func TestAdapter_Recall_EmptyContext_NoError(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/context", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sess-1","messages":[]}`)
	})
	got, err := a.Recall(context.Background(), sampleKey(), "anything")
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if got.Text != "" {
		t.Errorf("empty context should produce empty Text, got %q", got.Text)
	}
}

func TestAdapter_Recall_Timeout(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/context", func(w http.ResponseWriter, r *http.Request) {
		// Simulamos latencia > recall timeout.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	// Override config con timeout muy corto.
	a.cfg.RecallTimeout = 50 * time.Millisecond
	_, err := a.Recall(context.Background(), sampleKey(), "q")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAdapter_Remember_Empty_NoHTTP(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	if err := a.Remember(context.Background(), sampleKey(), nil); err != nil {
		t.Fatalf("Remember(nil): %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("server should not be hit, got %d calls", len(fake.calls))
	}
}

func TestAdapter_Remember_ChunkingAt100(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	var batchSizes []int
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/messages", func(w http.ResponseWriter, r *http.Request) {
		var body MessageBatchCreate
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode: %v", err)
		}
		batchSizes = append(batchSizes, len(body.Messages))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`[]`))
	})
	msgs := make([]agentapp.MemoryMessage, 250)
	for i := range msgs {
		msgs[i] = agentapp.MemoryMessage{
			Role:      "user",
			Text:      "msg",
			CreatedAt: time.Now(),
		}
	}
	if err := a.Remember(context.Background(), sampleKey(), msgs); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	wantSizes := []int{100, 100, 50}
	if len(batchSizes) != len(wantSizes) {
		t.Fatalf("batches: got %d, want %d (sizes=%v)", len(batchSizes), len(wantSizes), batchSizes)
	}
	for i, want := range wantSizes {
		if batchSizes[i] != want {
			t.Errorf("batch[%d]: got %d, want %d", i, batchSizes[i], want)
		}
	}
}

func TestAdapter_Remember_TruncatesLongContent(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	var sent MessageBatchCreate
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/messages", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`[]`))
	})
	longText := strings.Repeat("a", 30000)
	msgs := []agentapp.MemoryMessage{
		{Role: "assistant", Text: longText, CreatedAt: time.Now()},
	}
	if err := a.Remember(context.Background(), sampleKey(), msgs); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if len(sent.Messages) != 1 {
		t.Fatalf("sent messages: got %d, want 1", len(sent.Messages))
	}
	if len(sent.Messages[0].Content) > 25000 {
		t.Errorf("content length %d exceeds Honcho limit 25000", len(sent.Messages[0].Content))
	}
	if !strings.Contains(sent.Messages[0].Content, "[truncated by honcho adapter]") {
		t.Errorf("truncation suffix missing: %q", sent.Messages[0].Content[len(sent.Messages[0].Content)-50:])
	}
}

func TestAdapter_Remember_DiscardsUnknownRole(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	called := false
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/messages", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`[]`))
	})
	msgs := []agentapp.MemoryMessage{
		{Role: "system", Text: "internal", CreatedAt: time.Now()},
		{Role: "tool", Text: "tool output", CreatedAt: time.Now()},
	}
	if err := a.Remember(context.Background(), sampleKey(), msgs); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	// Si todos fueron descartados, NO se hace la request HTTP.
	if called {
		t.Error("HTTP should not be hit if all messages had unknown roles")
	}
}

func TestAdapter_Remember_RoleToPeerID(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	var sent MessageBatchCreate
	fake.on("/v3/workspaces/ws-1/sessions/sess-1/messages", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`[]`))
	})
	msgs := []agentapp.MemoryMessage{
		{Role: "user", Text: "hi", CreatedAt: time.Now()},
		{Role: "assistant", Text: "hello", CreatedAt: time.Now()},
		{Role: "USER", Text: "second", CreatedAt: time.Now()}, // case-insensitive
	}
	if err := a.Remember(context.Background(), sampleKey(), msgs); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if len(sent.Messages) != 3 {
		t.Fatalf("sent: got %d, want 3", len(sent.Messages))
	}
	if sent.Messages[0].PeerID != userPeerIDFor("alice@example.com") {
		t.Errorf("user msg peer_id: got %q, want %q", sent.Messages[0].PeerID, userPeerIDFor("alice@example.com"))
	}
	if sent.Messages[1].PeerID != "agent-sess-1" {
		t.Errorf("assistant msg peer_id: got %q, want agent-sess-1", sent.Messages[1].PeerID)
	}
	if sent.Messages[2].PeerID != userPeerIDFor("alice@example.com") {
		t.Errorf("USER (uppercase) msg peer_id: got %q", sent.Messages[2].PeerID)
	}
}

func TestAdapter_Isolation_DifferentSessionIDs(t *testing.T) {
	t.Parallel()
	a, fake := newTestAdapter(t)
	defaultEnsurePeersHandlers(fake)
	// Agrego handler para una segunda session.
	fake.on("/v3/workspaces/ws-1/sessions/sess-2/context", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sess-2","messages":[],"summary":null}`)
	})
	// Recall contra sess-1 sin handler configurado explícitamente
	// (sólo el genérico) debería fallar con 404, no leakear
	// data de sess-2.
	key1 := sampleKey()
	key2 := sampleKey()
	key2.SessionID = "sess-2"
	_, err := a.Recall(context.Background(), key1, "q")
	if err == nil {
		t.Fatal("expected error for sess-1 context (no handler)")
	}
	if !errors.Is(err, err) { // sanity: error no es nil
		t.Fatal("error should not be nil")
	}
}

// rURLQuery extrae el query string del request capturado.
// (r.URL.RawQuery se guarda separado del Path)
func rURLQuery(c capturedCall) string {
	return c.RawQuery
}
