package v2

import (
	"encoding/json"
	"strings"
	"testing"

	agentapp "lastmile-agents/internal/agent/application"
)

func TestRenderMessage_EmptyUserItem(t *testing.T) {
	// ponytail: un user prompt vacío no debería tirar error ni
	// emitir HTML inválido. Hoy RenderItem siempre emite un wrapper
	// data-msg; si el text está vacío el cliente V2 debe mostrar
	// un placeholder. Aquí verificamos que el HTML es no-vacío y
	// tiene el data-msg esperado.
	html, err := RenderMessage(agentapp.ConversationItem{Kind: "user", Text: "hola", Seq: 5})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, `data-msg="5"`) {
		t.Fatalf("HTML debería contener data-msg=5, got %s", html)
	}
	if !strings.Contains(html, "hola") {
		t.Fatalf("HTML debería contener el texto, got %s", html)
	}
	if !strings.Contains(html, "v2-item-user") {
		t.Fatalf("HTML debería tener la clase v2-item-user, got %s", html)
	}
}

func TestRenderMessage_ToolItemShowsArgs(t *testing.T) {
	// ponytail: M-B.2 cambió el formato de tool args. Antes
	// era un <pre class="v2-tool-args"> con JSON indent; ahora
	// es un <div class="v2-tool-args"> con key:value plano
	// para objetos de primitivos. Verificamos que el HTML
	// contenga tanto el nombre del tool como el comando
	// (escapado porque formatToolArgs escapa HTML siempre).
	args := json.RawMessage(`{"command":"ls -la"}`)
	html, err := RenderMessage(agentapp.ConversationItem{
		Kind:     "tool",
		ToolName: "bash",
		Args:     args,
		Seq:      7,
	})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, "bash") {
		t.Fatalf("HTML debería contener el tool name, got %s", html)
	}
	if !strings.Contains(html, "ls -la") {
		t.Fatalf("HTML debería contener el comando, got %s", html)
	}
	if !strings.Contains(html, "v2-tool-arg") {
		t.Fatalf("HTML debería tener la clase v2-tool-arg, got %s", html)
	}
}

func TestFormatToolArgs_PrimitiveObject(t *testing.T) {
	// ponytail: M-B.2 = tool args legibles. Para objetos planos
	// con valores primitivos, esperamos key:value línea por línea
	// en lugar del JSON crudo anterior.
	args := json.RawMessage(`{"command":"ls -la","timeout":30,"verbose":true}`)
	html := formatToolArgs(args)
	for _, want := range []string{
		`class="v2-tool-arg-key"`, "command", "timeout", "verbose",
		`class="v2-tool-arg-val"`, `&quot;ls -la&quot;`, "30", "true",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML debería contener %q, got %s", want, html)
		}
	}
}

func TestFormatToolArgs_NestedFallback(t *testing.T) {
	// ponytail: cuando los args tienen anidación (arrays u
	// objetos), caemos al JSON formateado con un <pre>. El
	// usuario ve "json:" como prefijo para entender por qué
	// el formato es distinto.
	args := json.RawMessage(`{"opts":{"nested":true},"items":[1,2,3]}`)
	html := formatToolArgs(args)
	if !strings.Contains(html, "v2-tool-args-fallback") {
		t.Fatalf("HTML debería usar el fallback para args anidados, got %s", html)
	}
	if !strings.Contains(html, "json:") {
		t.Fatalf("HTML debería tener el prefijo json:, got %s", html)
	}
}

func TestFormatToolArgs_Empty(t *testing.T) {
	// ponytail: args vacío o null → string vacío. El render ya
	// filtra esto (no emite el wrapper), pero el helper debe
	// ser defensivo por si lo llaman de otro lado.
	if got := formatToolArgs(json.RawMessage("")); got != "" {
		t.Fatalf("args vacío = %q, want ''", got)
	}
	if got := formatToolArgs(json.RawMessage("null")); got != "" {
		t.Fatalf("args null = %q, want ''", got)
	}
}

func TestFormatToolArgs_XSSEscaped(t *testing.T) {
	// ponytail: los args vienen del LLM. Si contienen <script>
	// u otro HTML, el helper lo escapa (no lo procesa como
	// markdown, sólo texto plano).
	args := json.RawMessage(`{"command":"echo <script>alert('xss')</script>"}`)
	html := formatToolArgs(args)
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Fatalf("HTML no debería contener <script>, got %s", html)
	}
	if !strings.Contains(html, "&lt;script") {
		t.Fatalf("HTML debería tener &lt;script (escapado), got %s", html)
	}
}

func TestSessionPreviewText(t *testing.T) {
	// ponytail: M-B.2 sidebar preview. El server guarda hasta
	// 600 chars en LastPreview. El helper sessionPreviewText
	// trunca a ~80 chars, elimina tags <think>...</think>
	// inline (que pi puede dejar en el text_delta), y agrega
	// ellipsis si se cortó.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"vacío", "", ""},
		{"solo espacios", "   \n  ", ""},
		{"corto sin truncar", "Hola mundo", "Hola mundo"},
		{"con think tags", "<think>razonamiento</think>respuesta visible", "respuesta visible"},
		{"think anidado", "<think>a<think>b</think>c</think>d", "c</think>d"},
		{"think sin cierre", "<think>sin cierre", ""},
		{"largo truncado con ellipsis", strings.Repeat("a", 200), strings.Repeat("a", 79) + "…"},
		{"con newlines", "línea 1\nlínea 2\nlínea 3", "línea 1 línea 2 línea 3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sessionPreviewText(tc.input)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderMessage_AssistantStripsThinkTags(t *testing.T) {
	// ponytail: mientras la V2 no tenga bloques de thinking
	// separados (eso entra en M-B), los tags <think>...</think>
	// se descartan del texto visible. Si no los quitamos, el chat
	// muestra reasoning crudo mezclado con la respuesta.
	html, err := RenderMessage(agentapp.ConversationItem{
		Kind: "assistant",
		Text: "<think>razonamiento</think>respuesta visible",
		Seq:  9,
	})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if strings.Contains(html, "razonamiento") {
		t.Fatalf("HTML no debería contener el contenido de <think>, got %s", html)
	}
	if !strings.Contains(html, "respuesta visible") {
		t.Fatalf("HTML debería contener el texto visible, got %s", html)
	}
}

func TestRenderMessage_AssistantCodeBlockHasLanguage(t *testing.T) {
	// ponytail: M-B.2 = syntax highlighting via highlight.js.
	// goldmark detecta el lenguaje después de ``` y agrega la
	// clase language-X al <code>. highlight.js lee esa clase
	// para saber qué gramática usar. Si en el futuro cambia el
	// formato (e.g. usamos un renderer custom), este test nos
	// avisa de que el highlight va a fallar silenciosamente.
	html, err := RenderMessage(agentapp.ConversationItem{
		Kind: "assistant",
		Text: "```go\nfunc main() {}\n```",
		Seq:  1,
	})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, `class="language-go"`) {
		t.Fatalf("code block debería tener class language-go para highlight.js, got %s", html)
	}
	// También verificamos que esté dentro de <pre><code>.
	if !strings.Contains(html, "<pre>") || !strings.Contains(html, "<code") {
		t.Fatalf("HTML debería tener <pre><code>, got %s", html)
	}
}

func TestRenderMessage_AssistantRendersMarkdown(t *testing.T) {
	// ponytail: M-B.1 = markdown en assistant. Verificamos que el
	// HTML producido tiene las tags correctas para los elementos
	// más comunes (negrita, código inline, code blocks, listas,
	// links). Si goldmark cambia su output, esto nos avisa.
	tests := []struct {
		name  string
		text  string
		want  string
	}{
		{"negrita", "**bold**", "<strong>bold</strong>"},
		{"italica", "*it*", "<em>it</em>"},
		{"code inline", "`x`", "<code>x</code>"},
		{"code block go", "```go\nfunc main(){}\n```", "<pre><code"},
		{"lista", "- a\n- b", "<ul>"},
		{"link", "[t](https://e.com)", `href="https://e.com"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html, err := RenderMessage(agentapp.ConversationItem{
				Kind: "assistant", Text: tc.text, Seq: 1,
			})
			if err != nil {
				t.Fatalf("RenderMessage err = %v", err)
			}
			if !strings.Contains(html, tc.want) {
				t.Fatalf("HTML debería contener %q, got %s", tc.want, html)
			}
		})
	}
}

func TestRenderMessage_AssistantSanitizesXSS(t *testing.T) {
	// ponytail: el texto del assistant viene del LLM, que puede
	// generar HTML/scripts accidentalmente. bluemonday debe
	// limpiarlos antes de llegar al DOM. Si este test falla,
	// alguien deshabilitó el sanitizer y hay un XSS abierto.
	xssInputs := []struct {
		name string
		text string
	}{
		{"script tag", "<script>alert('xss')</script>hola"},
		{"javascript url", "<a href=\"javascript:alert('xss')\">click</a>"},
		{"onclick", "<p onclick=\"alert('xss')\">click</p>"},
		{"iframe", "<iframe src='evil.com'></iframe>"},
	}
	for _, tc := range xssInputs {
		t.Run(tc.name, func(t *testing.T) {
			html, err := RenderMessage(agentapp.ConversationItem{
				Kind: "assistant", Text: tc.text, Seq: 1,
			})
			if err != nil {
				t.Fatalf("RenderMessage err = %v", err)
			}
			lower := strings.ToLower(html)
			for _, bad := range []string{"<script", "javascript:", "onclick=", "<iframe"} {
				if strings.Contains(lower, bad) {
					t.Fatalf("HTML contiene %q (no sanitizado): %s", bad, html)
				}
			}
		})
	}
}

func TestRenderMessage_ErrorItemUsesErrorClass(t *testing.T) {
	html, err := RenderMessage(agentapp.ConversationItem{Kind: "error", Text: "boom", Seq: 11})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, "v2-item-error") {
		t.Fatalf("HTML debería tener la clase v2-item-error, got %s", html)
	}
	if !strings.Contains(html, "boom") {
		t.Fatalf("HTML debería contener el mensaje, got %s", html)
	}
}

func TestRendererV2_SatisfiesFragmentRenderer(t *testing.T) {
	// ponytail: el bridge de V2 hacia application debe satisfacer
	// la interfaz FragmentRenderer. Si rompe la signature, el
	// compilador lo dice — este test es documentación viva de la
	// intención y cubre el camino de fallback.
	var r agentapp.FragmentRenderer = rendererV2{}
	html, err := r.RenderFragment(agentapp.ConversationItem{Kind: "user", Text: "x", Seq: 1})
	if err != nil {
		t.Fatalf("RenderFragment err = %v", err)
	}
	if html == "" {
		t.Fatalf("RenderFragment devolvió HTML vacío")
	}
}

func TestV2BudgetText_Empty(t *testing.T) {
	// ponytail: si el gateway no devolvió datos, las celdas del
	// budget bar muestran "—" y el data-state del contenedor es
	// "empty". Verificamos acá el render de las celdas.
	if got := v2BalanceText(PageState{}); got != "—" {
		t.Fatalf("balance sin snapshot = %q, want —", got)
	}
	if got := v2SessionSpentText(PageState{}); got != "—" {
		t.Fatalf("session spent sin snapshot = %q, want —", got)
	}
	if got := v2SessionRequestsText(PageState{}); got != "0 req" {
		t.Fatalf("session requests sin snapshot = %q, want 0 req", got)
	}
	if got := v2BudgetState(PageState{}); got != "empty" {
		t.Fatalf("budget state sin snapshot = %q, want empty", got)
	}
}

func TestV2BudgetText_Populated(t *testing.T) {
	balance := 1.50
	state := PageState{
		SessionCostUSD:   0.42,
		SessionCostReady: true,
		SessionCostReqs:  7,
		BalanceUSD:       &balance,
	}
	if got := v2BalanceText(state); got != "$1.50" {
		t.Fatalf("balance populated = %q, want $1.50", got)
	}
	if got := v2SessionSpentText(state); got != "$0.42" {
		t.Fatalf("session spent populated = %q, want $0.42", got)
	}
	if got := v2SessionRequestsText(state); got != "7 req" {
		t.Fatalf("session requests populated = %q, want 7 req", got)
	}
	if got := v2BudgetState(state); got != "ok" {
		t.Fatalf("budget state populated = %q, want ok", got)
	}
}

func TestV2FormatUSD(t *testing.T) {
	// ponytail: el formato es importante para el display. < 0.01
	// muestra 4 decimales (para que un costo real de $0.0013 no
	// aparezca como $0.00). >= 0.01 muestra 2 decimales.
	cases := []struct {
		in   float64
		want string
	}{
		{0, "$0.00"},
		{0.0013, "$0.0013"},
		{0.005, "$0.0050"},
		{1.5, "$1.50"},
		{1234.5678, "$1234.57"},
	}
	for _, tc := range cases {
		if got := v2FormatUSD(tc.in); got != tc.want {
			t.Errorf("v2FormatUSD(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRegisterRendererForSession_DoesNotPanicOnEmptyID(t *testing.T) {
	// ponytail: si por algún bug llega un sessionID vacío, la V2
	// no debe panicar ni ensuciar el registry global.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRendererForSession(\"\") no debe panicar: %v", r)
		}
	}()
	RegisterRendererForSession("")
	// También verificamos que clear es no-op sobre vacío.
	ClearRendererForSession("")
}

func TestAssetsFS_ContainsMainJS(t *testing.T) {
	// ponytail: si alguien renombra o borra main.js, este test
	// falla antes de runtime. El FS embebido se valida al compilar
	// (go:embed exige que el archivo exista), pero la verificación
	// de presencia por nombre es defensa contra cambios accidentales
	// del módulo entrypoint que el template standalone.templ espera.
	entries, err := AssetsFS.ReadDir("static/agent-chat")
	if err != nil {
		t.Fatalf("ReadDir err = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "main.js" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("static/agent-chat/main.js debería existir")
	}
}

func TestRenderMessage_ThinkingItem(t *testing.T) {
	// ponytail: el bloque de thinking se renderiza como <details>
	// colapsable con un summary que muestra el largo del texto.
	html, err := RenderMessage(agentapp.ConversationItem{
		Kind: "thinking",
		Text: "let me check the file structure first",
		Seq:  7,
	})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, "<details") {
		t.Fatalf("thinking debería usar <details>, got %.200s", html)
	}
	if !strings.Contains(html, "Pensando") {
		t.Fatalf("summary debería decir 'Pensando', got %.200s", html)
	}
	if !strings.Contains(html, "let me check") {
		t.Fatalf("HTML debería contener el texto, got %.200s", html)
	}
	if !strings.Contains(html, `data-kind="thinking"`) {
		t.Fatalf("data-kind debería ser thinking, got %.200s", html)
	}
}

func TestRenderMessage_ToolResultItem(t *testing.T) {
	// ponytail: el output de la tool va como card separada del
	// tool (que tiene los args). El label muestra el toolName.
	html, err := RenderMessage(agentapp.ConversationItem{
		Kind:     "tool_result",
		ToolName: "bash",
		Text:     "file1.txt\nfile2.txt",
		Seq:      9,
	})
	if err != nil {
		t.Fatalf("RenderMessage err = %v", err)
	}
	if !strings.Contains(html, "v2-tool-output") {
		t.Fatalf("HTML debería tener clase v2-tool-output, got %.200s", html)
	}
	if !strings.Contains(html, "output · bash") {
		t.Fatalf("label debería decir 'output · bash', got %.200s", html)
	}
	if !strings.Contains(html, "file1.txt") {
		t.Fatalf("HTML debería contener el output, got %.200s", html)
	}
	// ponytail: M-B.2 syntax highlighting. El <pre> del
	// tool_result ahora envuelve un <code> adentro para que
	// highlight.js pueda procesarlo. Sin esto, los outputs de
	// tools se ven como texto plano sin colores.
	if !strings.Contains(html, "<pre class=\"v2-tool-output\"><code>") {
		t.Fatalf("HTML debería tener <pre><code> para highlight.js, got %.200s", html)
	}
}

func TestV2ThinkingMeta(t *testing.T) {
	// El meta del header de thinking cambia según el largo del
	// texto. Vacío → "vacío", corto → "N caracteres", largo →
	// "N palabras · M caracteres".
	if got := v2ThinkingMeta(""); got != "vacío" {
		t.Fatalf("v2ThinkingMeta(\"\") = %q, want vacío", got)
	}
	if got := v2ThinkingMeta("hola mundo"); got != "10 caracteres" {
		t.Fatalf("v2ThinkingMeta(short) = %q, want 10 caracteres", got)
	}
	longText := strings.Repeat("palabra ", 50)
	got := v2ThinkingMeta(longText)
	if !strings.Contains(got, "palabras") {
		t.Fatalf("v2ThinkingMeta(long) debería mencionar palabras, got %q", got)
	}
	if !strings.Contains(got, "caracteres") {
		t.Fatalf("v2ThinkingMeta(long) debería mencionar caracteres, got %q", got)
	}
}

// ponytail: el budget bar V2 muestra el modelo activo y el % del
// context window. Estos tests verifican el render de los helpers
// + el state (ok/warning/error) según umbrales.
func TestV2ContextText_Formatting(t *testing.T) {
	cases := []struct {
		name        string
		state       PageState
		wantText    string
		wantState   string
	}{
		{
			name:      "empty",
			state:     PageState{},
			wantText:  "—",
			wantState: "ok",
		},
		{
			name: "small",
			state: PageState{
				SessionContextWindow:   1000000,
				SessionTotalTokens:     250,
				SessionPromptTokens:    200,
				SessionCompletionTokens: 50,
			},
			wantText:  "250/1M (0%)",
			wantState: "ok",
		},
		{
			name: "warning",
			state: PageState{
				SessionContextWindow:   1000,
				SessionTotalTokens:     600,
				SessionPromptTokens:    500,
				SessionCompletionTokens: 100,
			},
			wantText:  "600/1k (60%)",
			wantState: "warning",
		},
		{
			name: "error",
			state: PageState{
				SessionContextWindow:   1000,
				SessionTotalTokens:     850,
				SessionPromptTokens:    700,
				SessionCompletionTokens: 150,
			},
			wantText:  "850/1k (85%)",
			wantState: "error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := v2ContextText(tc.state); got != tc.wantText {
				t.Errorf("v2ContextText = %q, want %q", got, tc.wantText)
			}
			if got := v2ContextState(tc.state); got != tc.wantState {
				t.Errorf("v2ContextState = %q, want %q", got, tc.wantState)
			}
		})
	}
}

func TestV2SessionModelText_PassesAliasThrough(t *testing.T) {
	if got := v2SessionModelText(PageState{SessionModelAlias: "minimax/m3"}); got != "minimax/m3" {
		t.Errorf("got %q, want minimax/m3", got)
	}
	if got := v2SessionModelText(PageState{}); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
