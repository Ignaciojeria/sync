package v2

import (
	"bytes"
	"context"

	agentapp "lastmile-agents/internal/agent/application"
)

// rendererV2 satisface agentapp.FragmentRenderer usando el
// RenderMessage de la V2. El page handler llama a
// RegisterRendererForSession antes de servir la página, de modo
// que cuando llega un SSE event para esa sesión, el lookup por
// sessionID elige este renderer.
//
// Patrón: V2 no toca el fallback global (no existe más en el
// cutover de 2026-07); sólo registra por sesión. Si V1 y V2
// compartían renderer antes (V1 seteaba fallback), eso ya no
// aplica: la V1 dejó de existir.
type rendererV2 struct{}

func (rendererV2) RenderFragment(item agentapp.ConversationItem) (string, error) {
	return RenderMessage(item)
}

// RenderToolResultPartial produce el HTML para un update
// parcial de tool (tool_execution_update). A diferencia de
// RenderFragment (que va por el transcript), este usa el
// template RenderToolResultItem que añade data-upsert-key
// para que el cliente reemplace el mismo nodo DOM en cada
// update. El data-msg es estable ("tool_result:<toolCallID>")
// para que cualquier lógica de upsert por data-msg siga
// funcionando aunque la nueva por UpsertKey no esté
// desplegada en el cliente.
func (rendererV2) RenderToolResultPartial(toolCallID, toolName, text string) (string, error) {
	var buf bytes.Buffer
	if err := RenderToolResultItem(toolCallID, toolName, text).Render(context.Background(), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
