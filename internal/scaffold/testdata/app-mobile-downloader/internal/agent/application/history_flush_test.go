package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ponytail: tests para los fixes que cierran la pérdida de
// mensajes (ver internal/agent/application/history.go:181-260
// para flushDraftsIfPending y journalLastSeq).
//
// Cada test simula el patrón exacto que el user reportó:
//   - El transcript queda incompleto porque el runtime muere
//     antes de message_end (caso LRU eviction / abort / crash).
//   - El user refresca la página y ve el gap.
//   - Sin reiniciar el server, el gap debería cerrarse.

// TestFlushDraftsIfPending_RuntimeExitWithoutMessageEnd reproduce
// el bug más frecuente: message_start + text_deltas +
// runtime_exit (sin message_end). El draft del assistant debe
// llegar al transcript, no evaporarse.
func TestFlushDraftsIfPending_RuntimeExitWithoutMessageEnd(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"hola "}}`)})
	MaterializeEvent("s1", 3, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"mundo"}}`)})
	MaterializeEvent("s1", 4, Event{Type: "runtime_exit", Payload: json.RawMessage(`{"reason":"evicted"}`)})

	items, err := readConversationTranscript("s1")
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	// Esperamos 2 items: assistant (flushado por runtime_exit)
	// + error (que el case runtime_exit ya escribía).
	if len(items) != 2 {
		t.Fatalf("expected 2 items (assistant + error), got %d: %+v", len(items), items)
	}
	if items[0].Kind != "assistant" {
		t.Fatalf("items[0] = %+v, want assistant", items[0])
	}
	if items[0].Text != "hola mundo" {
		t.Fatalf("assistant text = %q, want %q", items[0].Text, "hola mundo")
	}
	if items[0].Seq != 2 {
		t.Fatalf("assistant seq = %d, want 2 (first text_delta)", items[0].Seq)
	}
	if items[1].Kind != "error" {
		t.Fatalf("items[1] = %+v, want error", items[1])
	}
}

// TestFlushDraftsIfPending_MessageStartOverrides verifica que el
// message_start de un turno NUEVO flushea los drafts del turno
// anterior antes de inicializar los nuevos. Antes del fix esto
// era un overwrite silencioso: el texto del assistant del turno
// anterior se perdía sin dejar rastro.
func TestFlushDraftsIfPending_MessageStartOverrides(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	// Turno A: message_start, unos text_deltas, turn_end sin
	// message_end (simulando la race que reporta el user).
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"respuesta A"}}`)})
	MaterializeEvent("s1", 3, Event{Type: "turn_end"})
	// Turno B: nuevo message_start debería flushear el draft de A.
	MaterializeEvent("s1", 10, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})

	items, _ := readConversationTranscript("s1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item (respuesta A), got %d: %+v", len(items), items)
	}
	if items[0].Kind != "assistant" || items[0].Text != "respuesta A" {
		t.Fatalf("items[0] = %+v, want assistant with 'respuesta A'", items[0])
	}
	if items[0].Seq != 2 {
		t.Fatalf("seq = %d, want 2", items[0].Seq)
	}
}

// TestFlushDraftsIfPending_AgentEndAndSettled cubren los otros
// dos eventos terminales para los que ahora tenemos flushing.
func TestFlushDraftsIfPending_AgentEndAndSettled(t *testing.T) {
	for _, evt := range []string{"agent_end", "agent_settled"} {
		t.Run(evt, func(t *testing.T) {
			defer resetRuntimeState(t)
			setTempDirs(t)
			MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
			MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"contenido"}}`)})
			MaterializeEvent("s1", 3, Event{Type: evt, Payload: json.RawMessage(`{}`)})
		})
	}
}

// TestFlushDraftsIfPending_NoDoubleFlush verifica el invariante
// clave del fix: si message_end ya flusheó, el runtime_exit/turn_end
// posterior es no-op (no debe escribir el mismo item dos veces).
func TestFlushDraftsIfPending_NoDoubleFlush(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"x"}}`)})
	MaterializeEvent("s1", 3, Event{Type: "message_end", Payload: json.RawMessage(`{"message":{"role":"assistant","content":[{"type":"text","text":"x"}]}}`)})
	MaterializeEvent("s1", 4, Event{Type: "turn_end"})
	MaterializeEvent("s1", 5, Event{Type: "agent_end"})

	items, _ := readConversationTranscript("s1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item (no duplicates), got %d: %+v", len(items), items)
	}
	if items[0].Text != "x" {
		t.Fatalf("text = %q, want x", items[0].Text)
	}
}

// TestFlushDraftsIfPending_RuntimeErrorDoesNotFlush verifica que
// runtime_error NO flushea (es potencialmente transitorio). Si
// llega un message_start después, ése sí flushea.
func TestFlushDraftsIfPending_RuntimeErrorDoesNotFlush(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"parcial"}}`)})
	// runtime_error transitorio: NO debe flushear el draft.
	MaterializeEvent("s1", 3, Event{Type: "runtime_error", Payload: json.RawMessage(`{"reason":"oops"}`)})

	transcriptState.Lock()
	_, hasAssistant := transcriptState.assistant["s1"]
	transcriptState.Unlock()
	if !hasAssistant {
		t.Fatalf("runtime_error shouldn't drain drafts; assistant draft should still be in map")
	}
}

// TestReadConversationTranscript_NoRebuildDespiteStaleJournal
// verifica la decisión arquitectónica de FIX #2 (revisado):
// NO rebuildamos el transcript desde el journal cuando hay
// gap. La rebuild agresivo mergeAssistantDelta-pisa los drafts
// en vivo del SSE handler y producía texto truncado (visto en
// agent-1784783206892149281-1 donde seq=717 assistant aparecía
// con 1422 chars truncados en una rebuild y 3571 chars completos
// en otra). En cambio: el transcript es source of truth para el
// chat; journal solo para replay manual. Si hay gap, FIX #1 +
// los flushes al final del turn cierran eventualmente las
// grietas mientras la sesión está activa.
func TestReadConversationTranscript_NoRebuildDespiteStaleJournal(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	// Transcript viejo con solo el user_prompt.
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	oldTranscript := `{"seq":1,"kind":"user","text":"hola"}
`
	if err := os.WriteFile(transcriptPath("s1"), []byte(oldTranscript), 0o640); err != nil {
		t.Fatal(err)
	}

	// Journal con eventos MUY adelante (Seq=200) — incluyendo un
	// message_start + text_delta + message_end con la respuesta
	// completa del assistant.
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	jPath := filepath.Join(eventsJournalDir(), "s1.jsonl")
	lines := []string{
		`{"seq":1,"kind":"pi","payload":{"type":"user_prompt","payload":{"text":"hola"}}}`,
		`{"seq":100,"kind":"pi","payload":{"type":"message_start"}}`,
		`{"seq":101,"kind":"pi","payload":{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"respuesta completa"}}}}`,
		`{"seq":102,"kind":"pi","payload":{"type":"message_end","payload":{"message":{"role":"assistant","content":[{"type":"text","text":"respuesta completa"}]}}}}`,
		`{"seq":200,"kind":"pi","payload":{"type":"turn_end"}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(jPath, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}

	items, err := readConversationTranscript("s1")
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	// Esperamos EXACTAMENTE lo que está en el transcript file:
	// sólo el user_prompt. NO reconstruimos desde el journal.
	if len(items) != 1 {
		t.Fatalf("expected 1 item (transcript file content), got %d: %+v",
			len(items), items)
	}
	if items[0].Kind != "user" || items[0].Text != "hola" {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

// TestJournalLastSeq_EmptyAndMissing cubre los casos borde:
// sessionID vacío y archivo inexistente.
func TestJournalLastSeq_EmptyAndMissing(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if seq := journalLastSeq(""); seq != 0 {
		t.Fatalf("empty session should yield 0, got %d", seq)
	}
	if seq := journalLastSeq("ghost-session"); seq != 0 {
		t.Fatalf("missing journal should yield 0, got %d", seq)
	}
}

// TestJournalLastSeq_MaxSeq verifies that the helper returns the
// maximum Seq regardless of insertion order in the journal.
func TestJournalLastSeq_MaxSeq(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	body := `{"seq":1,"kind":"pi","payload":{"type":"x"}}
{"seq":42,"kind":"pi","payload":{"type":"y"}}
{"seq":17,"kind":"pi","payload":{"type":"z"}}}
{"seq":42,"kind":"pi","payload":{"type":"w"}}
`
	_ = body
	body = `{"seq":1,"kind":"pi","payload":{"type":"x"}}
{"seq":42,"kind":"pi","payload":{"type":"y"}}
{"seq":17,"kind":"pi","payload":{"type":"z"}}
{"seq":42,"kind":"pi","payload":{"type":"w"}}
`
	if err := os.WriteFile(filepath.Join(eventsJournalDir(), "s1.jsonl"), []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	if seq := journalLastSeq("s1"); seq != 42 {
		t.Fatalf("lastSeq = %d, want 42", seq)
	}
}
