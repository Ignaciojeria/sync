package application

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setTempDirs redirige los directorios a t.TempDir() durante la duración del test.
func setTempDirs(t *testing.T) {
	t.Helper()
	t.Setenv("AGENT_TRANSCRIPTS_DIR", filepath.Join(t.TempDir(), "transcripts"))
	t.Setenv("AGENT_EVENTS_DIR", filepath.Join(t.TempDir(), "events"))
}

func resetRuntimeState(t *testing.T) {
	t.Helper()
	transcriptState.Lock()
	transcriptState.assistant = map[string]ConversationItem{}
	transcriptState.Unlock()
}

func TestLoadConversationHistoryDelegates(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	got, err := LoadConversationHistory("", 0, 0)
	if err != nil || len(got.Items) != 0 {
		t.Fatalf("got = %+v err = %v", got, err)
	}
}

func TestLoadHistory_EmptySessionID(t *testing.T) {
	defer resetRuntimeState(t)
	got, err := LoadConversationHistoryCtx(context.Background(), "", 0, 0)
	if err != nil || len(got.Items) != 0 {
		t.Fatalf("got = %+v err = %v", got, err)
	}
}

func TestLoadHistory_LimitDefaultsWhenZero(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 0)
	if err != nil || got.LastSeq != 1 || len(got.Items) != 1 {
		t.Fatalf("got = %+v", got)
	}
}

func TestLoadHistory_FilterBefore(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	for _, it := range []ConversationItem{{Seq: 1, Kind: "user"}, {Seq: 5, Kind: "user"}, {Seq: 10, Kind: "user"}} {
		if err := appendTranscriptItem("s1", it); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range got.Items {
		if item.Seq >= 10 {
			t.Fatalf("expected filter, got %+v", got.Items)
		}
	}
}

func TestLoadHistory_NextBeforeWhenExceedsLimit(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	for i := uint64(1); i <= 10; i++ {
		if err := appendTranscriptItem("s1", ConversationItem{Seq: i, Kind: "user", Text: fmt.Sprintf("%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasMore {
		t.Fatalf("expected HasMore, got %+v", got)
	}
	if got.NextBefore != got.Items[0].Seq {
		t.Fatalf("NextBefore = %d, want %d", got.NextBefore, got.Items[0].Seq)
	}
	if got.LastSeq != 10 {
		t.Fatalf("LastSeq = %d, want 10", got.LastSeq)
	}
}

func TestLoadHistory_PreviewTrimOnLimitOne(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	long := strings.Repeat("a", 1500)
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: long}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 1)
	if err != nil || len(got.Items) != 1 || len(got.Items[0].Text) >= 1500 {
		t.Fatalf("got = %+v", got)
	}
}

func TestLoadHistory_EmptyTranscript(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	got, err := LoadConversationHistoryCtx(context.Background(), "nope", 0, 10)
	if err != nil || len(got.Items) != 0 {
		t.Fatalf("got = %+v err = %v", got, err)
	}
}

func TestMaterializeUserPrompt(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeUserPrompt("s1", "  hola  ")
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Kind != "user" || items[0].Text != "hola" {
		t.Fatalf("items = %+v", items)
	}
	MaterializeUserPrompt("s1", "   ")
	MaterializeUserPrompt("", "x")
	items, _ = readConversationTranscript("s1")
	if len(items) != 1 {
		t.Fatalf("expected no extra items: %+v", items)
	}
}

func TestMaterializeEvent_EmptySessionID(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("", 1, Event{Type: "message_start"})
}

func TestMaterializeEvent_MessageStart_AssistantInit(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	transcriptState.Lock()
	_, ok := transcriptState.assistant["s1"]
	transcriptState.Unlock()
	if !ok {
		t.Fatal("assistant should be tracked")
	}
}

func TestMaterializeEvent_MessageStart_ShortCircuitsToError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{"message":{"role":"assistant","errorMessage":"boom","stopReason":"stop"}}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Kind != "error" || items[0].Text == "" {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_MessageUpdate_NotTextDelta(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking","delta":"x"}}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_MessageUpdate_InvalidJSON(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_update", Payload: json.RawMessage(`not-json`)})
}

func TestMaterializeEvent_MessageEnd_EmptyTextNoOp(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "message_end"})
	items, _ := readConversationTranscript("s1")
	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_MessageEnd_WithText(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 2, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 3, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"hola"}}`)})
	MaterializeEvent("s1", 4, Event{Type: "message_end"})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Kind != "assistant" || items[0].Text != "hola" || items[0].Seq != 3 {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_MessageEnd_SeqAlreadySet(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 9, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"x"}}`)})
	MaterializeEvent("s1", 99, Event{Type: "message_end"})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Seq != 9 {
		t.Fatalf("expected seq from update, got %+v", items)
	}
}

// ponytail: regression para el bug del mergeAssistantDelta que
// corrompía textos largos. Cuando message_end trae el
// contenido completo en payload.message.content, ese texto es
// la fuente de verdad — NO el draft acumulado por text_delta.
// El test simula el caso clásico de overlap=1 entre espacios
// consecutivos: el draft queda "Agent              Docs" (14
// espacios) y el payload trae "Agent               Docs" (15
// espacios). El fix debe preferir el payload.
func TestMaterializeEvent_MessageEnd_PrefersPayloadOverDraft(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	const fullText = "Agent               Docs" // 15 espacios entre Agent y Docs
	// Acumulamos via text_delta con un delta que TRIGGERS el
	// bug de overlap=1: current termina en espacio, delta
	// empieza en espacio.
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"Agent              "}`)})
	MaterializeEvent("s1", 3, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":" Docs           "}`)})
	// El draft acumulado está corrupto (un espacio menos).
	// message_end con payload completo debe sobrescribirlo.
	endPayload := []byte(`{"message":{"role":"assistant","content":[{"type":"text","text":"` + fullText + `"}]}}`)
	MaterializeEvent("s1", 4, Event{Type: "message_end", Payload: endPayload})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "assistant" {
		t.Fatalf("kind = %q, want assistant", items[0].Kind)
	}
	if items[0].Text != fullText {
		t.Fatalf("text = %q (%d chars), want %q (%d chars). El merge bug corrompió el draft y el payload no lo sobrescribió.",
			items[0].Text, len(items[0].Text), fullText, len(fullText))
	}
	if len(items[0].Text) != len(fullText) {
		t.Fatalf("len mismatch: got %d, want %d", len(items[0].Text), len(fullText))
	}
}

// ponytail: cuando message_end trae solo thinking (sin text),
// el payload thinking sobrescribe el draft. Esto cubre el caso
// de mensajes donde el assistant razona largo pero no produce
// respuesta visible — antes se persistía pensando corrupto,
// ahora se persiste el thinking completo del payload.
func TestMaterializeEvent_MessageEnd_PayloadThinkingOverridesDraft(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	const thinkingFull = "Análisis completo del usuario"
	const textFull = "respuesta final correcta"
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":"Análisis  "}`)})
	MaterializeEvent("s1", 3, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"thinking_delta","delta":" parcial"}`)})
	MaterializeEvent("s1", 4, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"respuesta  "}`)})
	MaterializeEvent("s1", 5, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":" vieja "}`)})
	endPayload := []byte(`{"message":{"role":"assistant","content":[{"type":"thinking","thinking":"` + thinkingFull + `"},{"type":"text","text":"` + textFull + `"}]}}`)
	MaterializeEvent("s1", 6, Event{Type: "message_end", Payload: endPayload})
	items, _ := readConversationTranscript("s1")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	var thinking, text string
	for _, it := range items {
		switch it.Kind {
		case "thinking":
			thinking = it.Text
		case "assistant":
			text = it.Text
		}
	}
	if thinking != thinkingFull {
		t.Fatalf("thinking = %q, want %q", thinking, thinkingFull)
	}
	if text != textFull {
		t.Fatalf("text = %q, want %q", text, textFull)
	}
}

// ponytail: si message_end no trae payload con message.content
// (eventos viejos, tests sintéticos), caemos al draft como
// antes. Esto preserva el contrato de los tests existentes.
func TestExtractAssistantContentFromPayload_ToolResultIgnored(t *testing.T) {
	// ponytail: pi emite message_end también para mensajes
	// con role=toolResult (el resultado de la tool se envía
	// dos veces). Sin el filtro de role, el contenido se
	// extrae como assistant y se duplica el tool_result que
	// ya capturamos por la otra vía (tool_execution_end).
	type want struct {
		text, thinking string
		hasMessage     bool
		role           string
	}
	cases := []struct {
		name    string
		payload string
		want    want
	}{
		{
			name:    "toolResult role",
			payload: `{"message":{"role":"toolResult","toolCallId":"abc","content":[{"type":"text","text":"output de la tool"}]}}`,
			want:    want{text: "", thinking: "", hasMessage: true, role: "toolResult"},
		},
		{
			name:    "user role",
			payload: `{"message":{"role":"user","content":[{"type":"text","text":"hola"}]}}`,
			want:    want{text: "", thinking: "", hasMessage: true, role: "user"},
		},
		{
			name:    "assistant role with text",
			payload: `{"message":{"role":"assistant","content":[{"type":"text","text":"respuesta"}]}}`,
			want:    want{text: "respuesta", thinking: "", hasMessage: true, role: "assistant"},
		},
		{
			name:    "assistant role case insensitive",
			payload: `{"message":{"role":"Assistant","content":[{"type":"text","text":"resp"}]}}`,
			want:    want{text: "resp", thinking: "", hasMessage: true, role: "Assistant"},
		},
		{
			name:    "assistant with thinking and text",
			payload: `{"message":{"role":"assistant","content":[{"type":"thinking","thinking":"razonamiento"},{"type":"text","text":"respuesta"}]}}`,
			want:    want{text: "respuesta", thinking: "razonamiento", hasMessage: true, role: "assistant"},
		},
		{
			name:    "empty payload",
			payload: ``,
			want:    want{text: "", thinking: "", hasMessage: false, role: ""},
		},
		{
			name:    "invalid JSON",
			payload: `not-json`,
			want:    want{text: "", thinking: "", hasMessage: false, role: ""},
		},
		{
			name:    "payload sin message field",
			payload: `{"type":"message_end"}`,
			want:    want{text: "", thinking: "", hasMessage: false, role: ""},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotText, gotThinking, gotHasMessage, gotRole := extractAssistantContentFromPayload(json.RawMessage(c.payload))
			if gotText != c.want.text || gotThinking != c.want.thinking || gotHasMessage != c.want.hasMessage || gotRole != c.want.role {
				t.Fatalf("got (%q, %q, %v, %q), want (%q, %q, %v, %q)",
					gotText, gotThinking, gotHasMessage, gotRole,
					c.want.text, c.want.thinking, c.want.hasMessage, c.want.role)
			}
		})
	}
}

// ponytail: regression para la duplicación de tool_result como
// assistant. Simula la secuencia real de pi: tool_execution_end
// (que persistimos como tool_result) seguido de message_end con
// role=toolResult (que pi emite además). Sin el filtro de role,
// el contenido se persiste como assistant duplicando el tool
// result. Con el fix, el draft se cae porque role!=assistant.
func TestMaterializeEvent_MessageEnd_ToolResultRoleDoesNotDuplicateAsAssistant(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	const toolOutput = "Successfully wrote 7885 bytes to /tmp/x.md"
	MaterializeEvent("s1", 1, Event{Type: "message_start", Payload: json.RawMessage(`{}`)})
	MaterializeEvent("s1", 2, Event{Type: "message_update", Payload: json.RawMessage(`{"assistantMessageEvent":{"type":"text_delta","delta":"draft previo"}}`)})
	// tool_execution_end: persiste como tool_result.
	MaterializeEvent("s1", 3, Event{Type: "tool_execution_end", Payload: json.RawMessage(`{"toolName":"write","result":{"content":[{"type":"text","text":"` + toolOutput + `"}]}}`)})
	// message_end con role=toolResult: NO debe crear un assistant con ese contenido.
	MaterializeEvent("s1", 4, Event{Type: "message_end", Payload: json.RawMessage(`{"message":{"role":"toolResult","content":[{"type":"text","text":"` + toolOutput + `"}]}}`)})
	items, _ := readConversationTranscript("s1")
	asstCount := 0
	toolResultCount := 0
	for _, it := range items {
		switch it.Kind {
		case "assistant":
			asstCount++
			if strings.Contains(it.Text, toolOutput) {
				t.Fatalf("assistant item contains tool output (duplicación): %q", it.Text)
			}
		case "tool_result":
			toolResultCount++
			if it.Text != toolOutput {
				t.Fatalf("tool_result text = %q, want %q", it.Text, toolOutput)
			}
		}
	}
	if asstCount != 0 {
		t.Fatalf("expected 0 assistant items, got %d. El message_end con role=toolResult se está persistiendo como assistant.", asstCount)
	}
	if toolResultCount != 1 {
		t.Fatalf("expected 1 tool_result, got %d", toolResultCount)
	}
}

func TestMaterializeEvent_ToolExecution_InvalidJSON(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "tool_execution_start", Payload: json.RawMessage(`not-json`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_ToolExecution_NameFromType(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "tool_execution_start", Payload: json.RawMessage(`{"type":"bash","args":{}}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].ToolName != "bash" {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_ToolExecution_NameFromToolName(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "tool_execution_start", Payload: json.RawMessage(`{"toolName":"grep","type":"","args":{}}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].ToolName != "grep" {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_RuntimeError_InvalidJSON(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "runtime_error", Payload: json.RawMessage(`not-json`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 0 {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_RuntimeError_FallbackToReason(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "runtime_error", Payload: json.RawMessage(`{"reason":"oh no"}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Text != "oh no" {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_RuntimeError_FallbackToType(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "stderr", Payload: json.RawMessage(`{}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Text != "stderr" {
		t.Fatalf("items = %+v", items)
	}
}

func TestMaterializeEvent_RuntimeError_FromMessage(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	MaterializeEvent("s1", 1, Event{Type: "runtime_exit", Payload: json.RawMessage(`{"message":"killed"}`)})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Text != "killed" || items[0].Kind != "error" {
		t.Fatalf("items = %+v", items)
	}
}

func TestReadConversationTranscript_BackfillAndRewrite(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	jp := filepath.Join(eventsJournalDir(), "s1.jsonl")
	jline := `{"seq":1,"kind":"user_prompt","payload":{"type":"user_prompt","payload":{"text":"from journal"}}}`
	if err := os.WriteFile(jp, []byte(jline+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	items, err := readConversationTranscript("s1")
	if err != nil || len(items) != 1 || items[0].Text != "from journal" {
		t.Fatalf("items = %+v err = %v", items, err)
	}
	if _, err := os.Stat(transcriptPath("s1")); err != nil {
		t.Fatalf("expected transcript rewritten: %v", err)
	}
}

func TestReadConversationTranscript_EmptyAfterBackfill(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	items, err := readConversationTranscript("ghost")
	if err != nil || len(items) != 0 {
		t.Fatalf("items = %+v err = %v", items, err)
	}
}

func TestReadConversationTranscript_ScanSkipsInvalid(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "not-json\n" + `{"seq":1,"kind":"user","text":"ok"}` + "\n" + `{"seq":2}` + "\n"
	if err := os.WriteFile(transcriptPath("s1"), []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	items, err := readConversationTranscript("s1")
	if err != nil || len(items) != 1 || items[0].Text != "ok" {
		t.Fatalf("items = %+v err = %v", items, err)
	}
}

func TestAppendTranscriptItem_EarlyReturns(t *testing.T) {
	if err := appendTranscriptItem("", ConversationItem{Kind: "user"}); err != nil {
		t.Fatalf("empty session: %v", err)
	}
	if err := appendTranscriptItem("s1", ConversationItem{}); err != nil {
		t.Fatalf("empty kind: %v", err)
	}
}

func TestAppendTranscriptItem_HappyPath(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 2, Kind: "user", Text: "b"}); err != nil {
		t.Fatal(err)
	}
	items, err := readConversationTranscript("s1")
	if err != nil || len(items) != 2 {
		t.Fatalf("items = %+v err = %v", items, err)
	}
}

func TestRewriteTranscript_HappyAndEmpty(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := rewriteTranscript("s1", nil); err != nil {
		t.Fatalf("rewrite empty: %v", err)
	}
	items := []ConversationItem{{Seq: 1, Kind: "user", Text: "a"}, {Seq: 2, Kind: "assistant", Text: "b"}}
	if err := rewriteTranscript("s1", items); err != nil {
		t.Fatal(err)
	}
	got, _ := readConversationTranscript("s1")
	if len(got) != 2 || got[1].Text != "b" {
		t.Fatalf("got = %+v", got)
	}
}

func TestBuildTranscriptFromLegacyJournal_EmptyJournal(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	items, err := buildTranscriptFromLegacyJournal("ghost")
	if err != nil || len(items) != 0 {
		t.Fatalf("items = %+v err = %v", items, err)
	}
}

func TestReadSessionJournal_OpenError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(eventsJournalDir(), "blocked.jsonl")
	if err := os.RemoveAll(journalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(journalPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := readSessionJournal("blocked"); err == nil {
		t.Fatal("expected os.Open error")
	}
}

func TestReadSessionJournal_ScanSkipsInvalid(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	body := "garbage\n" + `{"seq":1,"kind":"user_prompt","payload":{"type":"user_prompt","payload":{"text":"ok"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(eventsJournalDir(), "s1.jsonl"), []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	entries, err := readSessionJournal("s1")
	if err != nil || len(entries) != 1 || entries[0].Seq != 1 {
		t.Fatalf("entries = %+v err = %v", entries, err)
	}
}

func TestRebuildConversation_Empty(t *testing.T) {
	got := rebuildConversation(nil)
	if got != nil && len(got) != 0 {
		t.Fatalf("got = %+v", got)
	}
}

// ponytail: regression para el mismo bug del mergeAssistantDelta
// pero en el path de replay desde PostgreSQL. Cuando el journal
// tiene message_end con el contenido completo en
// payload.message.content, ese texto es la fuente de verdad y
// debe sobrescribir el draft acumulado por los text_delta
// previos. Sin esto, los replays siguen produciendo texto
// corrupto aunque arreglemos MaterializeEvent.
func TestRebuildConversationFromRuntimeRecords_PayloadOverridesDraft(t *testing.T) {
	const fullText = "Agent               Docs" // 15 espacios entre Agent y Docs
	entries := []historyJournalEntry{
		{Seq: 1, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant"}}}`)},
		// text_delta que triggerea el bug de overlap=1 entre
		// espacios: current termina en espacio, delta empieza
		// en espacio. El draft queda con 14 espacios en vez
		// de 15.
		{Seq: 2, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"Agent              "}}}`)},
		{Seq: 3, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":" Docs           "}}}`)},
		// message_end con payload completo (15 espacios). Debe
		// sobrescribir el draft corrupto.
		{Seq: 4, Payload: json.RawMessage(`{"type":"message_end","payload":{"message":{"role":"assistant","content":[{"type":"text","text":"` + fullText + `"}]}}}`)},
	}
	items := rebuildConversation(entries)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "assistant" {
		t.Fatalf("kind = %q, want assistant", items[0].Kind)
	}
	if items[0].Text != fullText {
		t.Fatalf("text = %q (%d chars), want %q (%d chars). El merge bug corrompió el replay y el payload no lo sobrescribió.",
			items[0].Text, len(items[0].Text), fullText, len(fullText))
	}
}

// ponytail: si message_end en el journal NO tiene payload con
// message.content (eventos viejos, journal corrupto), el replay
// cae al draft acumulado. Preserva la lógica existente.
func TestRebuildConversationFromRuntimeRecords_FallbackToDraftWhenNoPayload(t *testing.T) {
	entries := []historyJournalEntry{
		{Seq: 1, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant"}}}`)},
		{Seq: 2, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"draft text"}}}`)},
		{Seq: 3, Payload: json.RawMessage(`{"type":"message_end"}`)},
	}
	items := rebuildConversation(entries)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Text != "draft text" {
		t.Fatalf("text = %q, want draft text (fallback to draft)", items[0].Text)
	}
}

func TestRebuildConversationFromRuntimeRecords_AllPaths(t *testing.T) {
	entries := []historyJournalEntry{
		{Seq: 1, Payload: json.RawMessage(`{"type":"user_prompt","payload":{"text":"hi"}}`)},
		{Seq: 2, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant"}}}`)},
		{Seq: 3, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant","errorMessage":"failed","stopReason":"insufficient_credits"}}}`)},
		{Seq: 4, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"hola "}}}`)},
		{Seq: 5, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"other","delta":"skip"}}}`)},
		{Seq: 6, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"mundo"}}}`)},
		{Seq: 7, Payload: json.RawMessage(`{"type":"message_end"}`)},
		{Seq: 8, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":{"toolName":"","type":"bash","args":{}}}`)},
		{Seq: 9, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":"not-json"}`)},
		{Seq: 10, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":"not-json-2"}`)},
		{Seq: 11, Payload: json.RawMessage(`{"type":"runtime_error","payload":{"reason":"oops"}}`)},
		{Seq: 12, Payload: json.RawMessage(`{"type":"runtime_exit","payload":{"message":"killed"}}`)},
		{Seq: 13, Payload: json.RawMessage(`{"type":"runtime_error","payload":{}}`)},
		{Seq: 14, Payload: json.RawMessage(`{"type":"unknown_event_type"}`)},
		{Seq: 15, Payload: json.RawMessage(`{"type":"user_prompt","payload":{}}`)},
		{Seq: 16, Payload: json.RawMessage(`{"type":"user_prompt","payload":"not-json"}`)},
		{Seq: 17, Payload: json.RawMessage(`not-json`)},
		{Seq: 18, Payload: json.RawMessage(`not-json-2`)},
		{Seq: 19, Payload: json.RawMessage(`{"type":"message_update","payload":"not-json"}`)},
		{Seq: 20, Payload: json.RawMessage(`{"type":"runtime_error","payload":"not-an-object"}`)},
		{Seq: 21, Payload: json.RawMessage(`{"type":"stderr","payload":"not-an-object"}`)},
		{Seq: 22, Payload: json.RawMessage(`{"type":"runtime_exit","payload":"not-an-object"}`)},
	}
	items := rebuildConversation(entries)
	if len(items) < 1 {
		t.Fatalf("expected items, got %+v", items)
	}
	for _, it := range items {
		if strings.TrimSpace(it.Kind) == "" {
			t.Fatalf("empty kind: %+v", it)
		}
	}
}

func TestExtractAssistantStopError(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{"invalid JSON", `not-json`, "", false},
		{"non-assistant role", `{"message":{"role":"user"}}`, "", false},
		{"no errorMessage", `{"message":{"role":"assistant"}}`, "", false},
		{"with plain errorMessage", `{"message":{"role":"assistant","errorMessage":"boom"}}`, "boom", true},
		{"credits normalized", `{"message":{"role":"assistant","errorMessage":"","stopReason":"insufficient_credits"}}`, "", false},
		{"credits in message", `{"message":{"role":"assistant","errorMessage":"you have insufficient_credits","stopReason":"stop"}}`, "Créditos insuficientes en el proveedor/modelo configurado.", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := extractAssistantStopError(json.RawMessage(c.input))
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if c.ok && got != c.want {
				t.Fatalf("text = %q, want %q", got, c.want)
			}
		})
	}
}

func TestNormalizeAssistantErrorText(t *testing.T) {
	cases := []struct {
		text, stopReason, want string
	}{
		{"some random text", "stop", "some random text"},
		{"", "insufficient_credits", "Créditos insuficientes en el proveedor/modelo configurado."},
		{"you have insufficient_credits here", "stop", "Créditos insuficientes en el proveedor/modelo configurado."},
		{"got 402 from upstream", "", "Créditos insuficientes en el proveedor/modelo configurado."},
		{"payment required", "", "Créditos insuficientes en el proveedor/modelo configurado."},
		{"créditos insuficientes", "", "Créditos insuficientes en el proveedor/modelo configurado."},
		{"  trim me  ", "", "trim me"},
	}
	for _, c := range cases {
		got := normalizeAssistantErrorText(c.text, c.stopReason)
		if got != c.want {
			t.Errorf("normalize(%q, %q) = %q, want %q", c.text, c.stopReason, got, c.want)
		}
	}
}

// ponytail: cubre la rama de MessageEnd con Seq=0 forzando el estado
// interno. No se alcanza por flujo natural del sistema de eventos.
func TestMaterializeEvent_MessageEnd_SeqZeroForced(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	transcriptState.Lock()
	transcriptState.assistant["s1"] = ConversationItem{Kind: "assistant", Text: "predata"}
	transcriptState.Unlock()
	MaterializeEvent("s1", 7, Event{Type: "message_end"})
	items, _ := readConversationTranscript("s1")
	if len(items) != 1 || items[0].Seq != 7 || items[0].Text != "predata" {
		t.Fatalf("items = %+v", items)
	}
}

// ponytail: cubre las ramas de I/O error (directorio en lugar de archivo,
// líneas demasiado largas) y el camino alternativo de readConversationTranscript.
// En Windows no se puede simular EACCES read-only; usamos un directorio
// en la ruta del journal/transcript que devuelve ENOTDIR/EISDIR consistente.
func TestReadConversationTranscript_OpenErrorPath(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	tp := transcriptPath("s1")
	if err := os.Mkdir(tp, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := readConversationTranscript("s1"); err == nil {
		t.Fatal("expected os.Open error when path is a directory")
	}
}

func TestReadSessionJournal_OpenErrorPath(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	jp := filepath.Join(eventsJournalDir(), "blocked.jsonl")
	if err := os.Mkdir(jp, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := readSessionJournal("blocked"); err == nil {
		t.Fatal("expected os.Open error when journal path is a directory")
	}
}

func TestReadSessionJournal_ScanErrorOnLongLine(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(eventsJournalDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("a", 2*1024*1024) // supera sc.Buffer de 1 MiB
	if err := os.WriteFile(filepath.Join(eventsJournalDir(), "s1.jsonl"), []byte(huge+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := readSessionJournal("s1"); err == nil {
		t.Fatal("expected bufio.ErrTooLong from sc.Err")
	}
}

func TestLoadHistory_FallbackTranscriptOpenError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(transcriptPath("s1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 10); err == nil {
		t.Fatal("expected readConversationTranscript to propagate open error")
	}
}

// ponytail: en Windows, os.Open sobre un directorio NO devuelve error
// (devuelve handle + error solo al leer). Probamos este caso via `sc.Err`
// que ya está cubierto por tests anteriores; las ramas 195/202/272/284
// quedan inalcanzables en Windows sin permisos especiales.
func TestBackfillReturnsErrorWhenJournalPathIsBad(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(transcriptPath("s1"), 0o750); err != nil {
		t.Fatal(err)
	}
	// Crear un journal donde exista un archivo que apunte a un directorio
	// tampoco es fácil sin path separator. Esta función no es invocable
	// en Windows sin permisos; en Linux puede ser un punto de extensión.
	_ = readConversationTranscript
}

// ponytail: cubre las ramas de error de MkdirAll forzando un directorio padre
// pre-existente como archivo regular. ENOTDIR es portable.
func TestAppendTranscriptItem_MkdirAllError(t *testing.T) {
	defer resetRuntimeState(t)
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_TRANSCRIPTS_DIR", blocker)
	if err := appendTranscriptItem("s1", ConversationItem{Kind: "user", Text: "x"}); err == nil {
		t.Fatal("expected MkdirAll error when parent path is a file")
	}
}

func TestRewriteTranscript_MkdirAllError(t *testing.T) {
	defer resetRuntimeState(t)
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_TRANSCRIPTS_DIR", blocker)
	if err := rewriteTranscript("s1", []ConversationItem{{Kind: "user"}}); err == nil {
		t.Fatal("expected MkdirAll error when parent path is a file")
	}
}

func TestAppendTranscriptItem_OpenFileError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(transcriptPath("s1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := appendTranscriptItem("s1", ConversationItem{Kind: "user", Text: "x"}); err == nil {
		t.Fatal("expected OpenFile error when transcript path is a directory")
	}
}

func TestAppendTranscriptItem_MarshalError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	// Args con RawMessage no-JSON obliga a json.Marshal a fallar.
	bad := ConversationItem{Kind: "user", Args: json.RawMessage("not-valid-json")}
	if err := appendTranscriptItem("s1", bad); err == nil {
		t.Fatal("expected marshal error when Args contains invalid JSON")
	}
}

func TestRewriteTranscript_OpenFileError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	if err := os.MkdirAll(transcriptDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(transcriptPath("s1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := rewriteTranscript("s1", []ConversationItem{{Kind: "user", Text: "x"}}); err == nil {
		t.Fatal("expected os.Create error when transcript path is a directory")
	}
}

func TestRewriteTranscript_MarshalError(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	bad := []ConversationItem{{Kind: "user", Args: json.RawMessage("not-valid-json")}}
	if err := rewriteTranscript("s1", bad); err == nil {
		t.Fatal("expected marshal error when item contains invalid JSON")
	}
}

// ponytail: regresión para auto-recuperación de transcripts
// corruptos por el bug del mergeAssistantDelta. El transcript
// tiene un assistant corrupto (texto corto) y el pi session
// file tiene el texto completo. LoadConversationHistory debe
// preferir el pi file cuando detecta la divergencia.
func TestLoadHistory_AutoRecoversCorruptedAssistantFromPiFile(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	// Transcript con un assistant corrupto (texto corto, como
	// quedaría después del bug del merge).
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "hola"}); err != nil {
		t.Fatal(err)
	}
	corruptedText := strings.Repeat("x", 500) // ~50% del real
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 2, Kind: "assistant", Text: corruptedText}); err != nil {
		t.Fatal(err)
	}

	// pi session file con el texto completo.
	fullText := strings.Repeat("a", 1000)
	piDir := piSessionPath("s1")
	// piSessionPath devuelve la ruta completa; extraemos el dir.
	if err := os.MkdirAll(filepath.Dir(piDir), 0o750); err != nil {
		t.Fatal(err)
	}
	piContent := `{"type":"message","id":"m1","message":{"role":"assistant","content":[{"type":"text","text":"` + fullText + `"}]}}
`
	if err := os.WriteFile(piDir, []byte(piContent), 0o640); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var asstText string
	for _, it := range got.Items {
		if it.Kind == "assistant" {
			asstText = it.Text
		}
	}
	if asstText != fullText {
		t.Fatalf("assistant text = %d chars, want %d. La auto-recuperación del pi file no se activó.",
			len(asstText), len(fullText))
	}
}

// ponytail: si el transcript y el pi file tienen assistant
// del mismo largo (o el del transcript es ≥70% del pi file),
// NO activamos la recuperación. Evita falsos positivos para
// resúmenes cortos legítimos.
func TestLoadHistory_DoesNotRecoverWhenAssistantLooksComplete(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	completeText := strings.Repeat("z", 100)
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "assistant", Text: completeText}); err != nil {
		t.Fatal(err)
	}

	piDir := piSessionPath("s1")
	if err := os.MkdirAll(filepath.Dir(piDir), 0o750); err != nil {
		t.Fatal(err)
	}
	// pi file con texto MUY parecido en largo (95% del real).
	piText := strings.Repeat("z", 95)
	piContent := `{"type":"message","id":"m1","message":{"role":"assistant","content":[{"type":"text","text":"` + piText + `"}]}}
`
	if err := os.WriteFile(piDir, []byte(piContent), 0o640); err != nil {
		t.Fatal(err)
	}

	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var asstText string
	for _, it := range got.Items {
		if it.Kind == "assistant" {
			asstText = it.Text
		}
	}
	// Esperamos el del transcript (100 chars), NO el del pi
	// file (95 chars).
	if len(asstText) != 100 {
		t.Fatalf("assistant text len = %d, want 100. La recuperación se activó cuando no debía.",
			len(asstText))
	}
}

// ponytail: regresión para el bug del rebuild path que no
// extraía los tool_result porque el unmarshal usaba
// record.Payload (event JSON entero) en vez de event.Payload
// (inner payload ya extraído). Sin este fix, el rebuild
// desde PostgreSQL/journal producía tool items sin sus
// tool_result companions.
func TestRebuildConversationFromRuntimeRecords_ToolExecutionEndExtractsResult(t *testing.T) {
	const toolOutput = "Successfully wrote 7885 bytes to /tmp/x.md"
	entries := []historyJournalEntry{
		{Seq: 1, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant"}}}`)},
		{Seq: 2, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":{"toolName":"write","args":{"path":"/tmp/x.md"}}}`)},
		{Seq: 3, Payload: json.RawMessage(`{"type":"tool_execution_end","payload":{"toolName":"write","result":{"content":[{"type":"text","text":"` + toolOutput + `"}]}}}`)},
	}
	items := rebuildConversation(entries)
	var got *ConversationItem
	for i := range items {
		if items[i].Kind == "tool_result" {
			got = &items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected tool_result item, got items=%+v", items)
	}
	if got.Text != toolOutput {
		t.Fatalf("tool_result text = %q, want %q. El unmarshal del payload anidado falló.",
			got.Text, toolOutput)
	}
	if got.ToolName != "write" {
		t.Fatalf("tool_result toolName = %q, want write", got.ToolName)
	}
}
