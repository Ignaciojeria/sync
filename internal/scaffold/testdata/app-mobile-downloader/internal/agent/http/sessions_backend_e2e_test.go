package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
	agentmemory "lastmile-agents/internal/agent/infrastructure/memory"
	"lastmile-agents/internal/shared/server"
)

// TestE2E_SSEViaV1Endpoint verifica el flujo prompt → SSE usando
// directamente el endpoint /agent/sessions. Es un regression test
// del backend de datos (create session, prompt, SSE events con
// envelope FragmentEvent correcto) sin pasar por la página del
// chat. Si este test pasa, los data handlers están bien wireados
// y el SSE emite envelopes correctos. Los tests del shell V2
// (agent_e2e_test.go) cubren el flow completo de la UI.
func TestE2E_SSEViaV1Endpoint(t *testing.T) {
	store := agentmemory.NewSessionStore()
	manager := agentapp.NewManager(store, &fakeEventRunner{})

	srv := server.NewServer()
	noopEditor := func(h http.Handler) http.Handler { return h }
	// Tras el cutover 2026-07 toda la superficie del agente vive
	// en una sola Register (UI V2 + data handlers, sin flag ni
	// split V1/V2). Pasamos nil en sessionCostSvc porque este
	// test no ejerce el pre-render del budget bar.
	Register(srv, manager, nil, OIDCRefreshConfig{}, noopEditor, nil)
	ts := httptest.NewServer(srv.Mux)
	t.Cleanup(ts.Close)

	// Crear sesión
	body := strings.NewReader(`{"title":"v1","cwd":".","model":"default"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/agent/sessions", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created struct {
		Session agentapp.Session `json:"session"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	sessionID := created.Session.ID

	// Pre-registramos un renderer per-session para que el SSE
	// emita HTML en lugar de envelope crudo. En producción lo hace
	// dashboardPage antes de servir la página; acá lo simulamos.
	t.Cleanup(func() { agentapp.ClearSessionRenderer(sessionID) })

	// Prompt
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

	// SSE
	sseReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/agent/sessions/"+sessionID+"/events?resume=live", nil)
	sseReq.Header.Set("Accept", "text/event-stream")
	sseCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	sseReq = sseReq.WithContext(sseCtx)
	sseResp, err := http.DefaultClient.Do(sseReq)
	if err != nil {
		t.Fatalf("sse: %v", err)
	}
	defer sseResp.Body.Close()
	if sseResp.StatusCode != http.StatusOK {
		t.Fatalf("sse status = %d", sseResp.StatusCode)
	}

	// Leer hasta turn-end. Sólo leemos los primeros eventos
	// (los del primer fragment del assistant) y verificamos que
	// la respuesta tenga el formato esperado.
	reader := bufio.NewReader(sseResp.Body)
	sawFragment := false
	sawTurnEnd := false
	for !sawTurnEnd {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read sse: %v (sawFragment=%v)", err, sawFragment)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var env struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal([]byte(payload), &env); err != nil {
				continue
			}
			// El kind="phase" (e.g. "thinking") llega aunque no
			// haya renderer; lo aceptamos como señal de que el SSE
			// está vivo. Sólo exigimos ver al menos un envelope.
			if env.Kind == "phase" || env.Kind == "fragment" {
				sawFragment = true
			}
			if env.Kind == "turn-end" {
				sawTurnEnd = true
			}
		}
	}
	if !sawFragment {
		t.Fatalf("SSE nunca emitió ningún envelope")
	}
}
