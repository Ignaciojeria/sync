package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writePiSessionFile helper para tests: escribe un pi session
// file JSONL con el contenido dado. Devuelve el session ID y
// hace t.Cleanup para borrar el directorio.
func writePiSessionFile(t *testing.T, sessionID, content string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AGENT_PI_SESSIONS_DIR", dir)
	path := filepath.Join(dir, sessionID+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatalf("write pi session file: %v", err)
	}
	return sessionID
}

func TestBuildTranscriptFromPiSession_Empty(t *testing.T) {
	// ponytail: session ID vacío no debe tirar error, sólo
	// retorna nil (caller ya tiene fallback).
	if got := buildTranscriptFromPiSession(""); got != nil {
		t.Fatalf("empty sessionID = %v, want nil", got)
	}
}

func TestBuildTranscriptFromPiSession_FileNotFound(t *testing.T) {
	// ponytail: si el pi session file no existe, retorna nil
	// (no es un error — el caller cae al fallback de DB o
	// transcript vacío).
	t.Setenv("AGENT_PI_SESSIONS_DIR", t.TempDir())
	if got := buildTranscriptFromPiSession("nope"); got != nil {
		t.Fatalf("missing file = %v, want nil", got)
	}
}

func TestBuildTranscriptFromPiSession_UserAndAssistant(t *testing.T) {
	// ponytail: el caso típico. user prompt + assistant con
	// thinking + text. El helper debe:
	//   - Convertir user a kind="user" con el texto.
	//   - Separar thinking y text del assistant en dos items
	//     distintos (uno kind="thinking", otro kind="assistant").
	//   - Asignar seqs consecutivos para que el upsert del
	//     cliente V2 no los confunda.
	sessionID := writePiSessionFile(t, "sess-1", `{"type":"model_change","id":"m1"}
{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"hola"}]}}
{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"The user said hi"},{"type":"text","text":"¡Hola!"}]}}
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Kind != "user" || items[0].Text != "hola" {
		t.Fatalf("items[0] = %+v, want user 'hola'", items[0])
	}
	if items[0].Seq != 1 {
		t.Fatalf("items[0].Seq = %d, want 1", items[0].Seq)
	}
	if items[1].Kind != "thinking" || items[1].Text != "The user said hi" {
		t.Fatalf("items[1] = %+v, want thinking", items[1])
	}
	if items[2].Kind != "assistant" || items[2].Text != "¡Hola!" {
		t.Fatalf("items[2] = %+v, want assistant '¡Hola!'", items[2])
	}
	// ponytail: los seqs deben ser consecutivos (1, 2, 3) para
	// que el upsert del cliente V2 no sobrescriba el thinking
	// con el assistant (que tienen el mismo ID de mensaje en pi
	// pero son dos items distintos en el chat).
	if items[1].Seq != 2 || items[2].Seq != 3 {
		t.Fatalf("seqs = [%d, %d, %d], want [1, 2, 3]", items[0].Seq, items[1].Seq, items[2].Seq)
	}
}

func TestBuildTranscriptFromPiSession_ToolResult(t *testing.T) {
	// ponytail: toolResult del pi session file → kind="tool_result"
	// con toolName y texto.
	sessionID := writePiSessionFile(t, "sess-tool", `{"type":"message","id":"t1","message":{"role":"toolResult","toolCallId":"call_1","toolName":"bash","content":[{"type":"text","text":"hello world"}]}}
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Kind != "tool_result" {
		t.Fatalf("items[0].Kind = %q, want tool_result", items[0].Kind)
	}
	if items[0].ToolName != "bash" {
		t.Fatalf("items[0].ToolName = %q, want bash", items[0].ToolName)
	}
	if items[0].Text != "hello world" {
		t.Fatalf("items[0].Text = %q, want 'hello world'", items[0].Text)
	}
}

func TestBuildTranscriptFromPiSession_OnlyThinking(t *testing.T) {
	// ponytail: un assistant con SOLO thinking (sin text)
	// debe descartarse — el chat no muestra items vacíos.
	// En la práctica pi siempre emite text después del thinking,
	// pero cubrimos el edge case defensivamente.
	sessionID := writePiSessionFile(t, "sess-think", `{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"thinking","thinking":"solo thinking"}]}}
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 (only thinking, no text)", len(items))
	}
}

func TestBuildTranscriptFromPiSession_InvalidJSON(t *testing.T) {
	// ponytail: líneas con JSON inválido se descartan, no
	// matan el parseo entero.
	sessionID := writePiSessionFile(t, "sess-bad", `not json at all
{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"text","text":"hola"}]}}
also not json
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (only valid line)", len(items))
	}
}

func TestBuildTranscriptFromPiSession_AssistantWithoutThinking(t *testing.T) {
	// ponytail: assistant con sólo text (sin thinking) — debe
	// producir UN solo item kind="assistant".
	sessionID := writePiSessionFile(t, "sess-no-think", `{"type":"message","id":"a1","message":{"role":"assistant","content":[{"type":"text","text":"respuesta directa"}]}}
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Kind != "assistant" || items[0].Text != "respuesta directa" {
		t.Fatalf("items[0] = %+v, want assistant 'respuesta directa'", items[0])
	}
}

// ponytail: el user y el assistant que sólo tienen content no-text
// (sin text dentro) deben descartarse (no emitimos items vacíos).
func TestBuildTranscriptFromPiSession_EmptyContent(t *testing.T) {
	sessionID := writePiSessionFile(t, "sess-empty", `{"type":"message","id":"u1","message":{"role":"user","content":[{"type":"image","data":"..."}]}}
`)
	items := buildTranscriptFromPiSession(sessionID)
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 (no text content)", len(items))
	}
}

// ponytail: cleanMarkdownForPreview limpia el markdown del
// texto del LLM antes de guardarlo en Session.LastPreview.
// El sidebar del V2 muestra LastPreview en una sola línea
// truncada; sin esta limpieza, el preview muestra "**negrita**"
// en vez de "negrita".
func TestCleanMarkdownForPreview(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"vacío", "", ""},
		{"solo espacios", "   \n  ", ""},
		// Strip think tags inline.
		{"think tags", "<think>razonamiento</think>respuesta", "respuesta"},
		// Strip bold (**) — lo más común en respuestas del LLM.
		{"bold", "**negrita** texto", "negrita texto"},
		{"bold mixto", "**uno** y **dos**", "uno y dos"},
		// Strip italic.
		{"italic", "*italic* texto", "italic texto"},
		// Strip links → texto.
		{"link", "[click aquí](https://example.com)", "click aquí"},
		// Strip headings → texto sin #.
		{"heading", "# Título\n\nCuerpo", "Título Cuerpo"},
		// Strip code inline.
		{"code inline", "usá `foo()` para probar", "usá foo() para probar"},
		// Combinación: bold + link + italic.
		{"combo", "**Hola** [mundo](https://x.com) *fin*", "Hola mundo fin"},
		// List items: strip "- " prefix.
		{"lista", "- uno\n- dos", "uno dos"},
		// Blockquote: strip "> ".
		{"blockquote", "> cita", "cita"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanMarkdownForPreview(tc.input)
			if got != tc.want {
				t.Fatalf("cleanMarkdownForPreview(%q)\n  got:  %q\n  want: %q",
					tc.input, got, tc.want)
			}
		})
	}
}

// ponytail: cleanMarkdownForPreview debe colapsar whitespace
// excesivo para que el preview no tenga líneas vacías
// innecesarias (el sidebar es una sola línea).
func TestCleanMarkdownForPreview_CollapseWhitespace(t *testing.T) {
	input := "párrafo 1\n\n\n\n\npárrafo 2"
	want := "párrafo 1 párrafo 2"
	got := cleanMarkdownForPreview(input)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// ponytail: cleanMarkdownForPreview debe decodificar entidades
// HTML que goldmark genera en su output (por default usa
// &amp; en lugar de &, etc).
func TestCleanMarkdownForPreview_HtmlEntities(t *testing.T) {
	input := "fish &amp; chips"
	got := cleanMarkdownForPreview(input)
	if got != "fish & chips" {
		t.Fatalf("got %q, want %q (entities not decoded)", got, "fish & chips")
	}
}

// Referencia a json (para evitar import unused si sólo se usa
// en tests viejos).
var _ = json.RawMessage{}
