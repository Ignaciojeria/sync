package v2

import (
	"lastmile-agents/internal/agent/application"
)

// RegisterRendererForSession ata el renderer de V2 a una sesión.
// El page handler lo llama antes de renderizar la página, de
// modo que el SSE stream de esa sesión emita HTML en el formato
// V2. Cuando la sesión se elimina, el caller debe llamar a
// application.ClearSessionRenderer para liberar la entrada.
//
// Hoy el renderer V2 es único (rendererV2{}) y todos los tabs lo
// comparten. Si más adelante la V2 quiere variantes por sesión
// (tema custom, modo compact, etc.) se parametriza acá.
func RegisterRendererForSession(sessionID string) {
	application.SetSessionRenderer(sessionID, rendererV2{})
}

// ClearRendererForSession libera el renderer atado a una sesión.
// Simétrico con RegisterRendererForSession. Lo llama el handler
// de DELETE /agent/sessions/{id}.
func ClearRendererForSession(sessionID string) {
	application.ClearSessionRenderer(sessionID)
}
