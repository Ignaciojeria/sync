package application

import (
	"testing"
)

// TestExtractMessageUpdateDelta_NestedFormat: pi emite text_deltas
// con assistantMessageEvent anidado bajo event.Payload.payload,
// no al nivel superior. Antes del fix, este formato no se leía
// correctamente y los drafts quedaban vacíos durante streaming —
// el transcript file no recibía los text_deltas, sólo el
// message_end payload final.
func TestExtractMessageUpdateDelta_NestedFormat(t *testing.T) {
	// Formato anidado: {sessionId, type, payload: {type, assistantMessageEvent: {type, delta}}}
	nested := []byte(`{
		"sessionId": "s1",
		"type": "message_update",
		"payload": {
			"type": "message_update",
			"assistantMessageEvent": {
				"type": "text_delta",
				"delta": "Hola mundo"
			}
		}
	}`)

	gotType, gotText := extractMessageUpdateDelta(nested)
	if gotType != "text_delta" {
		t.Fatalf("type = %q, want text_delta", gotType)
	}
	if gotText != "Hola mundo" {
		t.Fatalf("delta = %q, want 'Hola mundo'", gotText)
	}
}

// TestExtractMessageUpdateDelta_FlatFormat: tolerar el formato
// viejo (assistantMessageEvent al nivel superior).
func TestExtractMessageUpdateDelta_FlatFormat(t *testing.T) {
	flat := []byte(`{
		"type": "message_update",
		"assistantMessageEvent": {
			"type": "thinking_delta",
			"delta": "Pensando..."
		}
	}`)

	gotType, gotText := extractMessageUpdateDelta(flat)
	if gotType != "thinking_delta" {
		t.Fatalf("type = %q, want thinking_delta", gotType)
	}
	if gotText != "Pensando..." {
		t.Fatalf("delta = %q, want 'Pensando...'", gotText)
	}
}

// TestExtractMessageUpdateDelta_EmptyAndGarbage: no retorna
// nada si el payload no matchea ningún formato.
func TestExtractMessageUpdateDelta_EmptyAndGarbage(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte(``),
		[]byte(`not json`),
		[]byte(`{"type": "message_update"}`), // no assistantMessageEvent en ninguno
		[]byte(`{"assistantMessageEvent": {"type": "", "delta": ""}}`),
	}
	for i, payload := range cases {
		gotType, gotText := extractMessageUpdateDelta(payload)
		if gotType != "" || gotText != "" {
			t.Fatalf("case %d: got (%q, %q), want empty", i, gotType, gotText)
		}
	}
}

// TestFlushDraftsIfPending_OnNewUserPrompt verifica que
// MaterializeUserPrompt flushea drafts residuales del assistant
// anterior. Este es el fix para "perdí parte de la respuesta
// al refrescar" — si el assistant estaba mid-stream cuando el
// user envió un nuevo prompt, los drafts parciales quedaban
// huérfanos hasta el próximo message_start del nuevo turn.
func TestFlushDraftsIfPending_OnNewUserPrompt(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	// Simulamos que el assistant acumuló texto antes del corte.
	MaterializeEvent("s1", 1, Event{Type: "message_start"})
	MaterializeEvent("s1", 2, Event{
		Type:    "message_update",
		Payload: []byte(`{"assistantMessageEvent":{"type":"text_delta","delta":"parcial"}}`),
	})

	// User envía nuevo prompt. MaterializeUserPrompt debe flushear
	// el draft 'parcial' al transcript Y escribir el nuevo user.
	MaterializeUserPrompt("s1", "nueva pregunta")

	items, _ := readConversationTranscript("s1")
	if len(items) != 2 {
		t.Fatalf("expected 2 items (assistant parcial + user), got %d: %+v",
			len(items), items)
	}
	// El primer item debe ser el assistant con texto 'parcial'.
	if items[0].Kind != "assistant" {
		t.Fatalf("items[0].kind = %q, want assistant", items[0].Kind)
	}
	if items[0].Text != "parcial" {
		t.Fatalf("items[0].text = %q, want 'parcial'", items[0].Text)
	}
	if items[0].Seq != 2 {
		t.Fatalf("items[0].seq = %d, want 2 (first text_delta)", items[0].Seq)
	}
	if items[1].Kind != "user" || items[1].Text != "nueva pregunta" {
		t.Fatalf("items[1] = %+v, want user 'nueva pregunta'", items[1])
	}

	// Después del flush + cleanup, los drafts DEBEN estar vacíos
	// para que el próximo turn arranque limpio.
	transcriptState.Lock()
	_, hasAssistant := transcriptState.assistant["s1"]
	_, hasThinking := transcriptState.thinking["s1"]
	transcriptState.Unlock()
	if hasAssistant || hasThinking {
		t.Fatalf("drafts should be cleared after MaterializeUserPrompt, "+
			"got hasAssistant=%v hasThinking=%v", hasAssistant, hasThinking)
	}
}
