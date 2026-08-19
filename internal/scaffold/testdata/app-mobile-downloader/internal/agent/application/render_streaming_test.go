package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRenderFragment_StreamingFirstDeltaEmits(t *testing.T) {
	// ponytail: el bug reportado en stable era que el cliente veía la
	// respuesta del assistant toda junta en lugar de progresiva. La
	// causa raíz estaba en streamOrSkip: si el draft del assistant
	// tenía Seq==0 (caso normal: message_start aún no fijó Seq),
	// descartaba el primer text_delta y sólo se emitía el HTML final
	// cuando llegaba message_end. Ahora asignamos Seq defensivamente
	// cuando todavía es 0, así el primer text_delta produce un
	// fragment con un id estable que el cliente hace upsert por
	// data-msg. Múltiples deltas comparten el id → upserts que
	// crecen visualmente.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})
	// Pre-poblamos el draft como si message_start hubiera llegado.
	MaterializeEvent("s1", 10, Event{Type: "message_start"})

	event := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"¡Pong!"}}`),
	}
	fragment, ok := RenderFragment("s1", 11, event)
	if !ok {
		t.Fatalf("primer text_delta debería emitir un fragment")
	}
	if fragment.Kind != "fragment" {
		t.Fatalf("fragment.Kind = %q, want fragment", fragment.Kind)
	}
	if fragment.ID == 0 {
		t.Fatalf("fragment.ID debería ser != 0 tras el fallback, got %d", fragment.ID)
	}
	if fragment.ItemKind != "assistant" {
		t.Fatalf("fragment.ItemKind = %q, want assistant", fragment.ItemKind)
	}
	if !strings.Contains(fragment.HTML, "¡Pong!") {
		t.Fatalf("fragment.HTML no contiene el delta: %s", fragment.HTML)
	}
}

func TestRenderFragment_SubsequentDeltaSameId(t *testing.T) {
	// El primer delta fija el Seq del draft. Los deltas subsiguientes
	// comparten ese id para que el cliente haga upsert por data-msg en
	// lugar de duplicar bubbles.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	event1 := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"Pa"}}`),
	}
	first, ok := RenderFragment("s1", 12, event1)
	if !ok {
		t.Fatalf("primer delta no emitió")
	}
	event2 := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"rt"}}`),
	}
	second, ok := RenderFragment("s1", 13, event2)
	if !ok {
		t.Fatalf("segundo delta no emitió")
	}
	if first.ID != second.ID {
		t.Fatalf("ambos deltas deberían compartir id: first=%d second=%d", first.ID, second.ID)
	}
	if !strings.Contains(second.HTML, "Part") {
		t.Fatalf("segundo delta acumuló texto: %s", second.HTML)
	}
}

// testFragmentRenderer es un FragmentRenderer mínimo para tests del
// package application — devuelve HTML con data-msg + data-kind para
// que el cliente pueda hacer upsert. La capa UI tiene su propio
// renderer (agentuiv2.RegisterRendererForSession) que se inyecta
// per-session via el page handler de la shell V2; este es suficiente
// para verificar la lógica de Seq y el contrato del envelope
// FragmentEvent.
type testFragmentRenderer struct{}

func (testFragmentRenderer) RenderFragment(item ConversationItem) (string, error) {
	if strings.TrimSpace(item.Text) == "" {
		return "", nil
	}
	return fmt.Sprintf(`<div data-msg="%d" data-kind="%s">%s</div>`,
		item.Seq, item.Kind, item.Text), nil
}

// RenderToolResultPartial es un stub simple para tests: emite
// HTML con data-upsert-key (el contrato que la UI V2 también
// sigue). El stub no es exacto byte-a-byte con la UI V2
// (no usamos el mismo template), pero cumple lo que el server
// necesita: HTML no vacío y (en este test) identificable.
func (testFragmentRenderer) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	return fmt.Sprintf(`<div data-msg="tool_result:%s" data-upsert-key="%s" data-kind="tool_result" class="v2-item v2-item-tool_result"><div class="v2-tool-output">%s</div></div>`,
		toolCallID, toolCallID, text), nil
}

func TestRenderFragment_ThinkingDeltaEmits(t *testing.T) {
	// ponytail: el segundo tipo de evento de pi es thinking_delta.
	// Antes se descartaba silenciosamente — ahora se mantiene en
	// transcriptState.thinking y se emite como fragment con
	// itemKind="thinking".
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	event := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":"let me check"}}`),
	}
	fragment, ok := RenderFragment("s2", 50, event)
	if !ok {
		t.Fatalf("thinking_delta debería emitir un fragment")
	}
	if fragment.ItemKind != "thinking" {
		t.Fatalf("ItemKind = %q, want thinking", fragment.ItemKind)
	}
	if !strings.Contains(fragment.HTML, "let me check") {
		t.Fatalf("HTML debería contener el delta, got %.200s", fragment.HTML)
	}
	if fragment.ID == 0 {
		t.Fatalf("fragment.ID debería ser != 0, got %d", fragment.ID)
	}
}

func TestRenderFragment_TextAndThinkingCoexist(t *testing.T) {
	// Un turno puede emitir thinking y text en paralelo (pi emite
	// ambos en el mismo message). Cada uno tiene su propio Seq para
	// que el upsert por data-msg no colisione.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	textEvent := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"respuesta"}}`),
	}
	thinkingEvent := Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":"pensando"}}`),
	}
	t1, _ := RenderFragment("s3", 60, textEvent)
	t2, _ := RenderFragment("s3", 61, thinkingEvent)
	if t1.ItemKind != "assistant" {
		t.Fatalf("text_delta → assistant, got %q", t1.ItemKind)
	}
	if t2.ItemKind != "thinking" {
		t.Fatalf("thinking_delta → thinking, got %q", t2.ItemKind)
	}
	if t1.ID == t2.ID {
		t.Fatalf("assistant y thinking deben tener Seq distinto, ambos en %d", t1.ID)
	}
}

func TestMaterializeEvent_FlushesBothDraftsOnMessageEnd(t *testing.T) {
	// ponytail: al cerrar message_end, flush de AMBOS drafts
	// (assistant + thinking). Si thinking quedó vacío, no se
	// materializa (no ensuciamos el feed con items vacíos).
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	// Inicializamos drafts como si message_start hubiera llegado.
	MaterializeEvent("s4", 100, Event{Type: "message_start"})

	// Enviamos un text_delta y un thinking_delta.
	MaterializeEvent("s4", 101, Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"hola"}}`),
	})
	MaterializeEvent("s4", 102, Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":"pensando..."}}`),
	})

	// message_end: ambos drafts deben materializarse.
	MaterializeEvent("s4", 103, Event{Type: "message_end"})

	history, err := LoadConversationHistory("s4", 0, 50)
	if err != nil {
		t.Fatalf("LoadConversationHistory err = %v", err)
	}

	var hasAssistant, hasThinking bool
	for _, item := range history.Items {
		if item.Kind == "assistant" && strings.Contains(item.Text, "hola") {
			hasAssistant = true
		}
		if item.Kind == "thinking" && strings.Contains(item.Text, "pensando") {
			hasThinking = true
		}
	}
	if !hasAssistant {
		t.Fatalf("assistant draft no se materializó: %+v", history.Items)
	}
	if !hasThinking {
		t.Fatalf("thinking draft no se materializó: %+v", history.Items)
	}
}

func TestMaterializeEvent_SkipsEmptyThinkingDraft(t *testing.T) {
	// Si thinking quedó vacío (preguntas simples sin razonamiento),
	// message_end NO debe materializar un item de thinking vacío.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	MaterializeEvent("s5", 200, Event{Type: "message_start"})
	MaterializeEvent("s5", 201, Event{
		Type:    "message_update",
		Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"ok"}}`),
	})
	// NO thinking_delta: el draft queda vacío.
	MaterializeEvent("s5", 202, Event{Type: "message_end"})

	history, _ := LoadConversationHistory("s5", 0, 50)
	for _, item := range history.Items {
		if item.Kind == "thinking" {
			t.Fatalf("no debería haber item de thinking vacío: %+v", item)
		}
	}
}

func TestMaterializeEvent_ToolExecutionEndCapturesResult(t *testing.T) {
	// ponytail: el output de una tool (bash, read, etc.) debe
	// materializarse como item aparte con kind="tool_result".
	// Antes se descartaba — el chat solo mostraba los args.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	MaterializeEvent("s6", 300, Event{
		Type: "tool_execution_start",
		Payload: json.RawMessage(`{
			"toolCallId":"call_1",
			"toolName":"bash",
			"args":{"command":"ls"}
		}`),
	})
	MaterializeEvent("s6", 301, Event{
		Type: "tool_execution_end",
		Payload: json.RawMessage(`{
			"toolCallId":"call_1",
			"toolName":"bash",
			"result":{"content":[{"type":"text","text":"file1.txt\nfile2.txt"}]}
		}`),
	})

	history, _ := LoadConversationHistory("s6", 0, 50)
	var hasTool, hasToolResult bool
	for _, item := range history.Items {
		if item.Kind == "tool" && item.ToolName == "bash" {
			hasTool = true
		}
		if item.Kind == "tool_result" && strings.Contains(item.Text, "file1.txt") {
			hasToolResult = true
		}
	}
	if !hasTool {
		t.Fatalf("tool item no se materializó: %+v", history.Items)
	}
	if !hasToolResult {
		t.Fatalf("tool_result item no se materializó: %+v", history.Items)
	}
}

func TestRenderFragment_ToolExecutionEndEmitsToolResult(t *testing.T) {
	// ponytail: tool_execution_end debe emitir el fragment del
	// output, no caer al fallback de EmitFragment que lee del
	// disco. Antes este evento se ignoraba completamente.
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	// Primero materializamos un tool start para que el seq del
	// tool_result no choque.
	MaterializeEvent("s10", 400, Event{
		Type:    "tool_execution_start",
		Payload: json.RawMessage(`{"toolName":"bash","args":{"command":"echo hi"}}`),
	})
	MaterializeEvent("s10", 401, Event{
		Type: "tool_execution_end",
		Payload: json.RawMessage(`{
			"toolCallId":"call_1",
			"toolName":"bash",
			"result":{"content":[{"type":"text","text":"hi\n"}]}
		}`),
	})

	// Verificamos que el item fue al transcript.
	history, err := LoadConversationHistory("s10", 0, 50)
	if err != nil {
		t.Fatalf("LoadConversationHistory err = %v", err)
	}
	var toolResult *ConversationItem
	for i := range history.Items {
		if history.Items[i].Kind == "tool_result" && history.Items[i].Seq == 401 {
			toolResult = &history.Items[i]
			break
		}
	}
	if toolResult == nil {
		t.Fatalf("tool_result no se materializó. Items: %+v", history.Items)
	}

	// Ahora RenderFragment debería encontrarlo.
	event := Event{
		Type:    "tool_execution_end",
		Payload: json.RawMessage(`{"toolCallId":"call_1","toolName":"bash"}`),
	}
	fragment, ok := RenderFragment("s10", 401, event)
	if !ok {
		t.Fatalf("tool_execution_end debería emitir un fragment")
	}
	if fragment.ItemKind != "tool_result" {
		t.Fatalf("ItemKind = %q, want tool_result", fragment.ItemKind)
	}
}

// TestRenderFragment_ToolExecutionUpdateStreamsPartialOutput cubre
// A1 (card de deprecar agent v1): el streaming de tool output
// vía tool_execution_update. Antes este evento se descartaba
// (caía al default del switch en RenderFragment), causando que
// tools largas (npm install, cargo build) parecieran "pegadas"
// durante segundos antes de mostrar el output final.
//
// El fix emite un FragmentEvent con:
//   - UpsertKey = toolCallId (estable a través de updates)
//   - HTML producido por renderer.RenderToolResultPartial
//   - NO materializa al transcript (las partials son transient)
//
// El cliente usa UpsertKey para reemplazar el mismo nodo DOM
// en cada update, así una tool con N updates crea UNA sola
// card que "crece" en vivo (no N cards).
func TestRenderFragment_ToolExecutionUpdateStreamsPartialOutput(t *testing.T) {
	defer resetRuntimeState(t)
	SetFragmentRenderer(testFragmentRenderer{})

	// Usamos una sessionID única por test (con sufijo random)
	// para no contaminar el transcript file en disco con
	// residuos de runs anteriores. Los tests del package
	// comparten el mismo tmp/agent-transcripts/, así que un
	// sessionID fijo (ej. "s_stream") acumularía items
	// previos y haría fallar los asserts.
	sessionID := fmt.Sprintf("stream-%d", time.Now().UnixNano())

	// tool_execution_update #1: pi emite el output acumulado.
	// El payload usa partialResult.content[0].text (no result).
	event1 := Event{
		Type: "tool_execution_update",
		Payload: json.RawMessage(`{
			"toolCallId": "call_42",
			"toolName": "bash",
			"partialResult": {
				"content": [{"type": "text", "text": "primera línea\n"}]
			}
		}`),
	}
	frag1, ok := RenderFragment(sessionID, 100, event1)
	if !ok {
		t.Fatalf("tool_execution_update #1 debería emitir un fragment, got ok=false")
	}
	if frag1.Kind != "fragment" {
		t.Errorf("Kind = %q, want fragment", frag1.Kind)
	}
	if frag1.ItemKind != "tool_result" {
		t.Errorf("ItemKind = %q, want tool_result", frag1.ItemKind)
	}
	if frag1.UpsertKey != "call_42" {
		t.Errorf("UpsertKey = %q, want call_42 (estable a través de updates)", frag1.UpsertKey)
	}
	if frag1.HTML == "" {
		t.Errorf("HTML vacío: el render del partial output falló")
	}

	// tool_execution_update #2: misma toolCallId, output acumulado
	// más largo. Mismo UpsertKey para que el cliente reemplace
	// el mismo nodo DOM.
	event2 := Event{
		Type: "tool_execution_update",
		Payload: json.RawMessage(`{
			"toolCallId": "call_42",
			"toolName": "bash",
			"partialResult": {
				"content": [{"type": "text", "text": "primera línea\nsegunda línea\n"}]
			}
		}`),
	}
	frag2, ok := RenderFragment(sessionID, 101, event2)
	if !ok {
		t.Fatalf("tool_execution_update #2 debería emitir un fragment")
	}
	if frag2.UpsertKey != "call_42" {
		t.Errorf("UpsertKey #2 = %q, want call_42 (DEBE ser estable)", frag2.UpsertKey)
	}
	if frag2.HTML == "" {
		t.Errorf("HTML vacío en update #2")
	}

	// tool_execution_end: el output final. Debe llevar el mismo
	// UpsertKey para que el cliente reemplace la card de streaming.
	MaterializeEvent(sessionID, 102, Event{
		Type: "tool_execution_end",
		Payload: json.RawMessage(`{
			"toolCallId": "call_42",
			"toolName": "bash",
			"result": {
				"content": [{"type": "text", "text": "primera línea\nsegunda línea\ntercera línea\n"}]
			}
		}`),
	})
	event3 := Event{
		Type: "tool_execution_end",
		Payload: json.RawMessage(`{
			"toolCallId": "call_42",
			"toolName": "bash",
			"result": {
				"content": [{"type": "text", "text": "primera línea\nsegunda línea\ntercera línea\n"}]
			}
		}`),
	}
	frag3, ok := RenderFragment(sessionID, 102, event3)
	if !ok {
		t.Fatalf("tool_execution_end debería emitir un fragment")
	}
	if frag3.UpsertKey != "call_42" {
		t.Errorf("UpsertKey del end = %q, want call_42 (DEBE ser estable para reemplazar la card de streaming)", frag3.UpsertKey)
	}

	// tool_execution_update con toolCallId ausente: debe descartar.
	eventNoID := Event{
		Type: "tool_execution_update",
		Payload: json.RawMessage(`{
			"toolName": "bash",
			"partialResult": {
				"content": [{"type": "text", "text": "sin toolCallId"}]
			}
		}`),
	}
	if _, ok := RenderFragment(sessionID, 110, eventNoID); ok {
		t.Errorf("tool_execution_update sin toolCallId debería descartarse (no hay UpsertKey estable)")
	}

	// tool_execution_update con partialResult vacío: debe descartar.
	eventEmpty := Event{
		Type: "tool_execution_update",
		Payload: json.RawMessage(`{
			"toolCallId": "call_x",
			"toolName": "bash",
			"partialResult": {
				"content": [{"type": "text", "text": ""}]
			}
		}`),
	}
	if _, ok := RenderFragment(sessionID, 111, eventEmpty); ok {
		t.Errorf("tool_execution_update con texto vacío debería descartarse")
	}
}
