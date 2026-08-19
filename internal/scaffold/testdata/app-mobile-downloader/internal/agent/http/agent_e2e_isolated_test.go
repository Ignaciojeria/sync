package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
	agentmemory "lastmile-agents/internal/agent/infrastructure/memory"
)

// TestE2E_RuntimeEmitsEventsViaManager es un test mínimo que
// verifica que el fake runtime emite eventos al SSE cuando el
// Manager recibe un prompt. Sirve para aislar problemas:
//   - Si este test falla, el problema está en el fake runtime o
//     en cómo el Manager reenvía eventos.
//   - Si este test pasa pero el TestE2E_HappyPath (en
//     agent_e2e_test.go) falla, el problema está en el wiring
//     HTTP o en los handlers del shell.
func TestE2E_RuntimeEmitsEventsViaManager(t *testing.T) {
	store := agentmemory.NewSessionStore()
	manager := agentapp.NewManager(store, &fakeEventRunner{})

	ctx := context.Background()

	// Crear sesión via Manager
	session, err := manager.Create(ctx, agentapp.CreateSessionInput{
		Title: "e2e",
		CWD:   ".",
		Model: "default",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Suscribirse ANTES del prompt (como hace el SSE handler).
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	events, _, err := manager.Subscribe(subCtx, session.ID)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Disparar el prompt
	if err := manager.Prompt(ctx, session.ID, "hola"); err != nil {
		t.Fatalf("prompt: %v", err)
	}

	// Esperar al menos un evento de tipo fragment.
	timeout := time.After(5 * time.Second)
	sawFragment := false
	sawTurnEnd := false
	for !sawTurnEnd {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("canal cerrado sin ver turn-end")
			}
			if ev.Type == "message_update" {
				// event llega crudo, no procesado por RenderFragment
				// (eso lo hace el SSE handler). Verificamos que el
				// text_delta está.
				var payload struct {
					AssistantMessageEvent struct {
						Type  string `json:"type"`
						Delta string `json:"delta"`
					} `json:"assistantMessageEvent"`
				}
				_ = json.Unmarshal(ev.Payload, &payload)
				if payload.AssistantMessageEvent.Type == "text_delta" {
					sawFragment = true
				}
			}
			if ev.Type == "message_end" || ev.Type == "agent_end" {
				sawTurnEnd = true
			}
		case <-timeout:
			t.Fatalf("timeout esperando eventos (sawFragment=%v)", sawFragment)
		}
	}
	if !sawFragment {
		t.Fatalf("no llegó ningún text_delta")
	}
}
