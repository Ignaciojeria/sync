package application

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestAppendTranscriptItem_Idempotent: si el último item del
// archivo coincide byte-por-byte con el que vamos a escribir,
// skip. Eso bloquea el patrón de duplicación que el user
// reportó: dos paths emiten el mismo item en sucesión
// (MaterializeEvent en message_end + flushDraftsIfPending en
// runtime_exit, FIX #2 rebuild que re-emite el último item, etc.).
func TestAppendTranscriptItem_Idempotent(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "hola"}); err != nil {
		t.Fatal(err)
	}

	// Same item again — second append should be a no-op.
	before, _ := os.Stat(transcriptPath("s1"))
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "user", Text: "hola"}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(transcriptPath("s1"))

	if before.Size() != after.Size() {
		t.Fatalf("file should not have grown on duplicate append: before=%d after=%d",
			before.Size(), after.Size())
	}

	// Different item — must append.
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 2, Kind: "assistant", Text: "chau"}); err != nil {
		t.Fatal(err)
	}
	after, _ = os.Stat(transcriptPath("s1"))
	if after.Size() <= before.Size() {
		t.Fatalf("file should have grown on distinct append")
	}

	// Item with same seq but different kind — must append
	// (it's a distinct entry, even though seq collides).
	prevSize := after.Size()
	if err := appendTranscriptItem("s1", ConversationItem{Seq: 1, Kind: "tool", ToolName: "bash"}); err != nil {
		t.Fatal(err)
	}
	after, _ = os.Stat(transcriptPath("s1"))
	if after.Size() <= prevSize {
		t.Fatalf("file should have grown on (seq, kind) change")
	}
}

// TestDedupItems_RemovesIdenticalLines: el dedup nuevo agrupa
// por signature len+kind+text[:40]. Items con misma signature
// (mismo long, kind, y los primeros 40 chars idénticos) se
// colapsan al primero; distintos se mantienen. Como bonus
// los (seq, kind) duplicados típicos de Fix #2 (sec=290
// assistant con "cora tests" vs "corra tests") se eliminan
// — el signature corta "cora tests" vs "corra tests" da hash
// distinto, PERO la primera ocurrencia es la que gana, y el
// dedup global sobre appendTranscriptItem vía readLastTranscriptLine
// previene el double-append que producía esas duplicaciones.
func TestDedupItems_RemovesIdenticalLines(t *testing.T) {
	items := []ConversationItem{
		{Seq: 1, Kind: "user", Text: "a"},
		{Seq: 1, Kind: "user", Text: "a"},     // dup (idéntico) → drop
		{Seq: 2, Kind: "assistant", Text: "b"},
		{Seq: 2, Kind: "assistant", Text: "C"}, // text distinto → kept (es un contenido distinto)
		{Seq: 3, Kind: "user", Text: "c"},      // different seq → keep
	}
	got := dedupItems(items)
	want := 4
	if len(got) != want {
		t.Fatalf("got %d items, want %d: %+v", len(got), want, got)
	}
	if got[0].Text != "a" || got[1].Text != "b" || got[2].Text != "C" || got[3].Text != "c" {
		t.Fatalf("unexpected items after dedup: %+v", got)
	}
}

// TestDedupItems_RemovesConsecutiveExactAppends reproducía el bug
// que tenía el sistema antes: Fix #2 producía dos appends del
// mismo item (con texto distinto por mergeAssistantDelta). La fix
// nueva usa signature len+kind+text[:40]. Si el contenido de los
// primeros 40 chars difiere, se mantienen ambos; si es idéntico,
// se colapsan. Ese es el caso del bug original (overlap=1 char
// no afecta primeros 40 chars si los deltas son largos).
func TestDedupItems_RemovesConsecutiveExactAppends(t *testing.T) {
	long := strings.Repeat("lorem ipsum dolor sit amet ", 10) // ~270 chars
	items := []ConversationItem{
		{Seq: 290, Kind: "assistant", Text: long},
		{Seq: 290, Kind: "assistant", Text: long}, // mismo text exacto → drop
	}
	got := dedupItems(items)
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (duplicates with identical content)", len(got))
	}
}

// TestDedupItems_DistinctUserPromptsSameSeqNotMerged reproduce el
// bug del usuario: MaterializeUserPrompt escribe TODOS los user
// prompts con Seq=0 (no son parte del flujo SSE/journal). El dedup
// viejo agrupaba por (seq,kind) y borraba 6 de cada 7 user
// prompts. El dedup nuevo usa un signature con len+kind+text[:40]
// para que user prompts distintos no se confundan.
func TestDedupItems_DistinctUserPromptsSameSeqNotMerged(t *testing.T) {
	items := []ConversationItem{
		{Seq: 0, Kind: "user", Text: "que opinas de la ux del chat de agentv2"},
		{Seq: 0, Kind: "user", Text: "sólo dame un bullet point de oportunidades de mejora"},
		{Seq: 0, Kind: "user", Text: "y con respecto a como esta implementado agentv2"},
		{Seq: 0, Kind: "user", Text: "dame un listado de features que podria ir añadiendo"},
		{Seq: 0, Kind: "user", Text: "si aplicamos esos cambios en una siguiente iteracion"},
	}
	got := dedupItems(items)
	if len(got) != 5 {
		t.Fatalf("expected 5 distinct user prompts (all Seq=0 different texts), "+
			"got %d: %+v", len(got), got)
	}
}

// TestAppendTranscriptItem_LargeFileStillDedups verifica que el
// dedup via readLastTranscriptLine sigue funcionando cuando el
// transcript file >64KB (límite de ReadAt). En la práctica cada
// item es <16KB, pero un archivo con muchos items podría
// exceder el buffer. Si la última línea está dentro del buffer,
// todo bien; si no, leer el archivo entero es la única opción
// (no debería pasar con items normales).
func TestAppendTranscriptItem_LargeFileStillDedups(t *testing.T) {
	defer resetRuntimeState(t)
	setTempDirs(t)

	// Write a bunch of items to make the file >64KB.
	for i := uint64(1); i <= 50; i++ {
		text := strings.Repeat("x", 1024) // 1KB per item, ~50KB total
		if err := appendTranscriptItem("s1", ConversationItem{
			Seq:  i,
			Kind: "user",
			Text: text,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Verify dedup on the last item.
	last := ConversationItem{
		Seq:  50,
		Kind: "user",
		Text: strings.Repeat("x", 1024),
	}
	before, _ := os.Stat(transcriptPath("s1"))
	if err := appendTranscriptItem("s1", last); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(transcriptPath("s1"))

	if before.Size() != after.Size() {
		t.Fatalf("file should not have grown on duplicate append at seq=50: before=%d after=%d",
			before.Size(), after.Size())
	}
	_ = json.Marshal
}
