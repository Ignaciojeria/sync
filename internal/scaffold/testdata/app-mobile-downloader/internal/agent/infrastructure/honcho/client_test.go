package honcho

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestClient crea un Client apuntando a un httptest.Server
// que ejecuta el handler provisto. Devuelve además el server
// para que el test pueda inspeccionar requests recibidos.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "test-key", &http.Client{Timeout: 2 * time.Second}), srv
}

// assertRequest verifica método, path prefix y Authorization.
func assertRequest(t *testing.T, r *http.Request, wantMethod, wantPathPrefix string) {
	t.Helper()
	if r.Method != wantMethod {
		t.Errorf("method: got %s, want %s", r.Method, wantMethod)
	}
	if !strings.HasPrefix(r.URL.Path, wantPathPrefix) {
		t.Errorf("path: got %s, want prefix %s", r.URL.Path, wantPathPrefix)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization: got %q, want %q", got, "Bearer test-key")
	}
	if r.Method == http.MethodPost && r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", r.Header.Get("Content-Type"))
	}
}

func TestClient_CreateWorkspace_OK(t *testing.T) {
	t.Parallel()
	var gotBody WorkspaceCreate
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/v3/workspaces")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := client.CreateWorkspace(context.Background(), "my-ws"); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if gotBody.ID != "my-ws" {
		t.Errorf("body.id: got %q, want my-ws", gotBody.ID)
	}
}

func TestClient_CreateWorkspace_422(t *testing.T) {
	t.Parallel()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":[{"loc":["body","id"],"msg":"invalid"}]}`))
	})
	err := client.CreateWorkspace(context.Background(), "")
	if err == nil {
		t.Fatal("expected error on 422")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 422 {
		t.Errorf("StatusCode: got %d, want 422", apiErr.StatusCode)
	}
}

func TestClient_GetOrCreatePeer_OK(t *testing.T) {
	t.Parallel()
	var gotPath string
	var gotBody PeerCreate
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		assertRequest(t, r, http.MethodPost, "/v3/workspaces/ws-1/peers")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := client.GetOrCreatePeer(context.Background(), "ws-1", "agent-x"); err != nil {
		t.Fatalf("GetOrCreatePeer: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/peers") {
		t.Errorf("path: got %s, want suffix /peers", gotPath)
	}
	if gotBody.ID != "agent-x" {
		t.Errorf("body.id: got %q, want agent-x", gotBody.ID)
	}
}

func TestClient_CreateSession_OK_WithPeers(t *testing.T) {
	t.Parallel()
	var gotBody SessionCreate
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/v3/workspaces/ws-1/sessions")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	peers := map[string]SessionPeerConfig{
		"agent-x":   {ObserveMe: false},
		"user-y":    {ObserveMe: true},
	}
	if err := client.CreateSession(context.Background(), "ws-1", "sess-1", peers); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if gotBody.ID != "sess-1" {
		t.Errorf("body.id: got %q, want sess-1", gotBody.ID)
	}
	if len(gotBody.Peers) != 2 {
		t.Errorf("body.peers: got %d, want 2", len(gotBody.Peers))
	}
	if gotBody.Peers["user-y"].ObserveMe != true {
		t.Errorf("user-y observe_me: got %v, want true", gotBody.Peers["user-y"].ObserveMe)
	}
	if gotBody.Peers["agent-x"].ObserveMe != false {
		t.Errorf("agent-x observe_me: got %v, want false", gotBody.Peers["agent-x"].ObserveMe)
	}
}

func TestClient_AddPeerToSession_OK(t *testing.T) {
	t.Parallel()
	var gotBody AddPeerToSessionRequest
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/v3/workspaces/ws-1/sessions/sess-1/peers")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	})
	if err := client.AddPeerToSession(context.Background(), "ws-1", "sess-1", "user-y", SessionPeerConfig{ObserveMe: true}); err != nil {
		t.Fatalf("AddPeerToSession: %v", err)
	}
	if gotBody.PeerID != "user-y" || !gotBody.ObserveMe {
		t.Errorf("body: got %+v, want peer_id=user-y observe_me=true", gotBody)
	}
}

func TestClient_GetSessionContext_OK(t *testing.T) {
	t.Parallel()
	var gotQuery url.Values
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodGet, "/v3/workspaces/ws-1/sessions/sess-1/context")
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id": "sess-1",
			"messages": [],
			"summary": null,
			"peer_representation": "user likes concise answers",
			"peer_card": ["backend dev", "es-ES"]
		}`)
	})
	got, err := client.GetSessionContext(context.Background(), "ws-1", "sess-1", GetSessionContextOptions{
		Tokens:     1000,
		SearchQuery: "qué dijo el agente",
		SearchTopK: 8,
		PeerTarget: "agent-x",
	})
	if err != nil {
		t.Fatalf("GetSessionContext: %v", err)
	}
	if gotQuery.Get("tokens") != "1000" {
		t.Errorf("query.tokens: got %q, want 1000", gotQuery.Get("tokens"))
	}
	if gotQuery.Get("search_query") != "qué dijo el agente" {
		t.Errorf("query.search_query: got %q", gotQuery.Get("search_query"))
	}
	if gotQuery.Get("search_top_k") != "8" {
		t.Errorf("query.search_top_k: got %q", gotQuery.Get("search_top_k"))
	}
	if gotQuery.Get("peer_target") != "agent-x" {
		t.Errorf("query.peer_target: got %q", gotQuery.Get("peer_target"))
	}
	if gotQuery.Get("summary") != "true" {
		t.Errorf("query.summary: got %q, want true (explicit)", gotQuery.Get("summary"))
	}
	if got.PeerRepresentation == nil || *got.PeerRepresentation != "user likes concise answers" {
		t.Errorf("peer_representation: got %v", got.PeerRepresentation)
	}
	if len(got.PeerCard) != 2 || got.PeerCard[0] != "backend dev" {
		t.Errorf("peer_card: got %v", got.PeerCard)
	}
}

func TestClient_GetSessionContext_EmptyQuery(t *testing.T) {
	t.Parallel()
	// Con Tokens=0 no se debe mandar el query param; el resto
	// tampoco porque están vacíos. Summary igual va explícito.
	var gotQuery url.Values
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"sess-1","messages":[]}`)
	})
	if _, err := client.GetSessionContext(context.Background(), "ws-1", "sess-1", GetSessionContextOptions{}); err != nil {
		t.Fatalf("GetSessionContext: %v", err)
	}
	if _, ok := gotQuery["tokens"]; ok {
		t.Errorf("tokens should NOT be sent when 0, got %v", gotQuery["tokens"])
	}
	if _, ok := gotQuery["search_query"]; ok {
		t.Errorf("search_query should NOT be sent when empty")
	}
}

func TestClient_CreateMessages_OK(t *testing.T) {
	t.Parallel()
	var gotBody MessageBatchCreate
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertRequest(t, r, http.MethodPost, "/v3/workspaces/ws-1/sessions/sess-1/messages")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	})
	msgs := []MessageCreate{
		{Content: "hola", PeerID: "user-y", CreatedAt: "2026-07-19T01:00:00Z"},
		{Content: "chau", PeerID: "agent-x", CreatedAt: "2026-07-19T01:00:05Z"},
	}
	if err := client.CreateMessages(context.Background(), "ws-1", "sess-1", msgs); err != nil {
		t.Fatalf("CreateMessages: %v", err)
	}
	if len(gotBody.Messages) != 2 {
		t.Errorf("body.messages len: got %d, want 2", len(gotBody.Messages))
	}
	if gotBody.Messages[0].PeerID != "user-y" {
		t.Errorf("body.messages[0].peer_id: got %q", gotBody.Messages[0].PeerID)
	}
}

func TestClient_CreateMessages_Empty_NoOp(t *testing.T) {
	t.Parallel()
	called := false
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	if err := client.CreateMessages(context.Background(), "ws-1", "sess-1", nil); err != nil {
		t.Fatalf("CreateMessages(nil): %v", err)
	}
	if called {
		t.Error("server should not be hit for empty batch")
	}
}

func TestClient_CreateMessages_BatchSizeLimit(t *testing.T) {
	t.Parallel()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be hit for over-sized batch")
	})
	msgs := make([]MessageCreate, 101)
	if err := client.CreateMessages(context.Background(), "ws-1", "sess-1", msgs); err == nil {
		t.Fatal("expected error for batch > 100")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	t.Parallel()
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulamos latencia: mientras el server espera, el client cancela.
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ya cancelado antes de llamar
	err := client.CreateWorkspace(ctx, "x")
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}
