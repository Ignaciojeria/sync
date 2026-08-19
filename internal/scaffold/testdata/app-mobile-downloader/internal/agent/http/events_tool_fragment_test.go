package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// TestMain is defined in events_sse_test.go for the legacy SSE stream
// tests. This file uses the same TestMain.

// ponytail: regression test para el bug de scroll durante tool
// execution. Antes el SSE handler hacía `continue` después de mandar
// el envelope de phase para tool_execution_start, saltándose
// EmitFragment. Resultado: el tool card (recién materializado al
// transcript) NUNCA llegaba al cliente durante streaming. Sólo
// aparecía después de un page reload, donde LoadConversationHistory
// lo recuperaba del transcript. Eso explicaba por qué el chat se
// quedaba visualmente arriba cuando el agente ejecutaba tools: el
// feed no crecía, el sticky scroll no tenía target.
//
// El fix: dejar que EmitFragment corra después de phase/turn-end,
// sólo skipeándolo cuando RenderFragment ya emitió un envelope con
// HTML (kind=fragment). El cliente recibe 2 envelopes por tool
// execution: phase (badge) + fragment (HTML del tool card).
//
// Este test verifica que tras tool_execution_start el SSE emite:
//   1. Un envelope phase {kind:"phase",phase:"tooling"}
//   2. Un envelope fragment con el HTML del tool card recién
//      materializado (data-msg con el seq del evento).
func TestStreamEvents_ToolExecutionStartEmitsFragmentNotOnlyPhase(t *testing.T) {
	if testing.Short() {
		t.Skip("skip SSE end-to-end test in short mode")
	}

	// Renderer específico para este test que sabe renderizar tool
	// items (el testFragmentRenderer genérico devuelve "" para
	// items sin texto, lo cual enmascara este caso de prueba).
	agentapp.SetFragmentRenderer(toolCardTestRenderer{})
	defer agentapp.SetFragmentRenderer(nil)

	stub := &streamEventsServiceStub{
		ch:     make(chan agentapp.Event, 16),
		cancel: func() {},
	}
	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/tool-fix-test/events?resume=live", nil)
	req.SetPathValue("id", "tool-fix-test")
	rr := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		streamEvents(stub)(rr, req)
	}()

	waitForInitialStatus(t, rr)

	env := agentapp.Event{
		Type:      "tool_execution_start",
		SessionID: "tool-fix-test",
		Payload:   []byte(`{"toolName":"bash","type":"bash","args":{"command":"ls"}}`),
	}

	time.Sleep(50 * time.Millisecond)
	stub.ch <- env

	time.Sleep(200 * time.Millisecond)

	close(stub.ch)
	<-done

	body := rr.Body.String()
	if testing.Verbose() {
		t.Logf("SSE body:\n%s", body)
	}
	envelopes := parseAgentFragmentEnvelopes(t, body)

	sawPhase := false
	sawFragment := false
	for _, env := range envelopes {
		switch env["kind"] {
		case "phase":
			if env["phase"] == "tooling" {
				sawPhase = true
			}
		case "fragment":
			sawFragment = true
			if id, ok := env["id"].(float64); !ok || id == 0 {
				t.Fatalf("fragment envelope sin id (data-msg) — el cliente no podría hacer upsert: %+v", env)
			}
			html, _ := env["html"].(string)
			if !strings.Contains(html, "data-msg") {
				t.Fatalf("fragment envelope sin HTML con data-msg: %+v", env)
			}
			if !strings.Contains(html, "tool") {
				t.Fatalf("fragment envelope debería tener la marca tool (data-kind=tool o class v2-item-tool), got: %s", html)
			}
		}
	}
	if !sawPhase {
		t.Fatalf("no se emitió el envelope phase=tooling; body:\n%s", body)
	}
	if !sawFragment {
		t.Fatalf("no se emitió el fragment del tool card (regression del fix de scroll); body:\n%s", body)
	}
	_ = json.RawMessage{}
	_ = context.Background
}

// toolCardTestRenderer emite HTML reconocible para items tipo tool
// además de los items tipo assistant. El testFragmentRenderer
// genérico (definido en events_sse_test.go) devuelve "" para items
// sin texto, lo cual enmascara este caso de prueba — necesitamos un
// renderer que Sí renderice el tool card para que EmitFragment
// pueda transmitir su HTML. Si devuelve "", EmitFragment emite un
// envelope con HTML vacío y el test no puede distinguir "fix
// funciona, renderer vacío" de "fix roto, no emite fragment".
type toolCardTestRenderer struct{}

func (toolCardTestRenderer) RenderFragment(item agentapp.ConversationItem) (string, error) {
	if item.Kind == "tool" || item.Kind == "tool_result" {
		return fmt.Sprintf(`<div data-msg="%d" data-kind="%s" data-tool-name="%s">TOOL CARD</div>`,
			item.Seq, item.Kind, item.ToolName), nil
	}
	if item.Text == "" {
		return "", nil
	}
	return fmt.Sprintf(`<div data-msg="%d" data-kind="%s">%s</div>`,
		item.Seq, item.Kind, item.Text), nil
}

func (toolCardTestRenderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	return "", nil
}
