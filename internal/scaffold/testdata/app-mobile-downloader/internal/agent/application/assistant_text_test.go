package application

import (
	"strings"
	"testing"
)

// TestVisibleAssistantText_StripsThinkBlocks cubre el happy path:
// bloques <think>...</think> bien formados se eliminan del texto
// visible. El thinking "real" se renderiza en su propio item
// kind="thinking"; esto es defensa por si el modelo inlinea tags
// en un text_delta.
func TestVisibleAssistantText_StripsThinkBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no tags", "Hola mundo", "Hola mundo"},
		{"empty", "", ""},
		{"only whitespace", "   \n  ", ""},
		{"complete block", "<think>razonamiento</think>respuesta", "respuesta"},
		{"complete block at end", "<think>razonamiento</think>", ""},
		{"complete block at start", "<think>razonamiento</think>respuesta", "respuesta"},
		{"complete block in middle", "antes<think>medio</think>después", "antesdespués"},
		{"two blocks", "<think>uno</think>A<think>dos</think>B", "AB"},
		{"tag before/after text", "<think>razonamiento</think>", ""},
		// ponytail: nested <think> NO se procesa bien (la impl
		// actual con strings.Index matchea el primer </think>
		// que encuentra, lo que deja el contenido "outer"
		// huérfano). Es un edge case raro (pi no emite nested
		// think blocks). Lo documentamos con el output actual
		// para que un cambio futuro lo note.
		{"nested (edge case, known imperfect)", "<think><think>inner</think>outer</think>x", "outerx"},
		// unclosed <think>: el stream se cortó. La fix: NO
		// borrar el texto que vino después (sólo el tag).
		{"unclosed at end", "<think>razonamiento parcial", "razonamiento parcial"},
		{"unclosed with text after", "<think>parcial", "parcial"},
		{"text then unclosed", "antes<think>parcial", "antesparcial"},
		{"only unclosed tag", "<think>", ""},
		{"only unclosed close tag", "</think>", ""},
		{"orphan close tag in middle", "antes</think>después", "antesdespués"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := visibleAssistantText(tc.in)
			if got != tc.want {
				t.Errorf("visibleAssistantText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestVisibleAssistantText_PreservesUnclosedThinkingContent es
// el regression test del bug que el user reportó como "texto
// español recortado". El comportamiento viejo (text = text[:start])
// borraba desde el <think> sin cierre hasta el final del string,
// perdiendo la respuesta visible que vino después. La fix
// preserva el texto restante aunque haya un <think> sin
// cerrar.
func TestVisibleAssistantText_PreservesUnclosedThinkingContent(t *testing.T) {
	// Caso real: el LLM empezó a razonar (<think>) y se cortó
	// el stream. El texto que viene después es la respuesta
	// visible que el usuario SÍ debería ver.
	input := "<think>empezando a razonar sobre la pregunta del usuario..."
	if got, want := visibleAssistantText(input), "empezando a razonar sobre la pregunta del usuario..."; got != want {
		t.Errorf("unclosed <think> al inicio: visibleAssistantText(%q) = %q, want %q", input, got, want)
	}

	// Caso: el LLM inlineó un <think> en el medio de su
	// respuesta, sin cerrarlo. El texto visible antes Y
	// después debe preservarse.
	input = "Esto es la respuesta<think>ahora interrumpo con un tag suelto y sigo"
	if got, want := visibleAssistantText(input), "Esto es la respuestaahora interrumpo con un tag suelto y sigo"; got != want {
		t.Errorf("unclosed <think> en el medio: visibleAssistantText(%q) = %q, want %q", input, got, want)
	}

	// Caso adversario: solo el tag <think> suelto. Resultado
	// esperado: string vacío (no hay nada que preservar).
	if got, want := visibleAssistantText("<think>"), ""; got != want {
		t.Errorf("solo <think>: visibleAssistantText(%q) = %q, want %q", "<think>", got, want)
	}

	// Caso: respuesta completa, con <think> al medio, sin
	// cerrar. El prefijo y el sufijo se preservan, el tag se va.
	input = "principio<think>mitad sin cerrarfinal"
	got := visibleAssistantText(input)
	if !strings.Contains(got, "principio") || !strings.Contains(got, "final") {
		t.Errorf("visibleAssistantText(%q) = %q, debería contener 'principio' y 'final'", input, got)
	}
}
