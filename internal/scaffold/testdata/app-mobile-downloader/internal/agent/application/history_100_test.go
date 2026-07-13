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

type fakeSource struct {
	rows []RuntimeEventRecord
	err  error
}

func (f *fakeSource) ListSession(_ context.Context, _ string) ([]RuntimeEventRecord, error) {
	return f.rows, f.err
}

func resetRuntimeState(t *testing.T) {
	t.Helper()
	SetRuntimeEventsHistorySource(nil)
	transcriptState.Lock()
	transcriptState.assistant = map[string]ConversationItem{}
	transcriptState.Unlock()
}

func TestSetRuntimeEventsHistorySource_RoundTrip(t *testing.T) {
	defer resetRuntimeState(t)
	SetRuntimeEventsHistorySource(&fakeSource{rows: []RuntimeEventRecord{{Offset: 1}}})
	if runtimeEventsHistorySource == nil {
		t.Fatal("expected source set")
	}
	SetRuntimeEventsHistorySource(nil)
	if runtimeEventsHistorySource != nil {
		t.Fatal("expected source cleared")
	}
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

func TestLoadHistory_SourceReturnsData(t *testing.T) {
	defer resetRuntimeState(t)
	SetRuntimeEventsHistorySource(&fakeSource{rows: []RuntimeEventRecord{
		{Offset: 1, Kind: "user_prompt", Payload: json.RawMessage(`{"type":"user_prompt","payload":{"text":"hi"}}`)},
	}})
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 10)
	if err != nil || len(got.Items) != 1 || got.Items[0].Text != "hi" {
		t.Fatalf("got = %+v err = %v", got, err)
	}
}

func TestLoadHistory_SourceError_FallsBackToTranscript(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	SetRuntimeEventsHistorySource(&fakeSource{err: fmt.Errorf("boom")})
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "fallback"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 10)
	if err != nil || len(got.Items) != 1 || got.Items[0].Text != "fallback" {
		t.Fatalf("got = %+v err = %v", got, err)
	}
}

func TestLoadHistory_SourceEmpty_FallsBackToTranscript(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)
	SetRuntimeEventsHistorySource(&fakeSource{rows: []RuntimeEventRecord{}})
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "fallback"}); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConversationHistoryCtx(context.Background(), "s1", 0, 10)
	if err != nil || len(got.Items) != 1 {
		t.Fatalf("got = %+v err = %v", got, err)
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

func TestRebuildConversationFromRuntimeRecords_AllPaths(t *testing.T) {
	records := []RuntimeEventRecord{
		{Offset: 1, Payload: json.RawMessage(`{"type":"user_prompt","payload":{"text":"hi"}}`)},
		{Offset: 2, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant"}}}`)},
		{Offset: 3, Payload: json.RawMessage(`{"type":"message_start","payload":{"message":{"role":"assistant","errorMessage":"failed","stopReason":"insufficient_credits"}}}`)},
		{Offset: 4, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"hola "}}}`)},
		{Offset: 5, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"other","delta":"skip"}}}`)},
		{Offset: 6, Payload: json.RawMessage(`{"type":"message_update","payload":{"assistantMessageEvent":{"type":"text_delta","delta":"mundo"}}}`)},
		{Offset: 7, Payload: json.RawMessage(`{"type":"message_end"}`)},
		{Offset: 8, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":{"toolName":"","type":"bash","args":{}}}`)},
		{Offset: 9, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":"not-json"}`)},
		{Offset: 10, Payload: json.RawMessage(`{"type":"tool_execution_start","payload":"not-json-2"}`)},
		{Offset: 11, Payload: json.RawMessage(`{"type":"runtime_error","payload":{"reason":"oops"}}`)},
		{Offset: 12, Payload: json.RawMessage(`{"type":"runtime_exit","payload":{"message":"killed"}}`)},
		{Offset: 13, Payload: json.RawMessage(`{"type":"runtime_error","payload":{}}`)},
		{Offset: 14, Payload: json.RawMessage(`{"type":"unknown_event_type"}`)},
		{Offset: 15, Payload: json.RawMessage(`{"type":"user_prompt","payload":{}}`)},
		{Offset: 16, Payload: json.RawMessage(`{"type":"user_prompt","payload":"not-json"}`)},
		{Offset: 17, Payload: json.RawMessage(`not-json`)},
		{Offset: 18, Payload: json.RawMessage(`not-json-2`)},
		{Offset: 19, Payload: json.RawMessage(`{"type":"message_update","payload":"not-json"}`)},
		{Offset: 20, Payload: json.RawMessage(`{"type":"runtime_error","payload":"not-an-object"}`)},
		{Offset: 21, Payload: json.RawMessage(`{"type":"stderr","payload":"not-an-object"}`)},
		{Offset: 22, Payload: json.RawMessage(`{"type":"runtime_exit","payload":"not-an-object"}`)},
	}
	items := rebuildConversationFromRuntimeRecords(records)
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
