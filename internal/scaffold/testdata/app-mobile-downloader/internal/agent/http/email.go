package agent

import (
	"net/http"
	"strings"

	"lastmile-agents/internal/auth/middleware"
)

// emailFromContext extrae el email del JWT en el request
// context. Devuelve "" si no hay claims o si el email está
// vacío. Usado por los handlers de session (create) y
// prompt (cada turn) para que el Manager pueda construir la
// MemoryKey del provider de memoria.
func emailFromContext(r *http.Request) string {
	claims, ok := middleware.JWTClaimsFromContext(r.Context())
	if !ok {
		return ""
	}
	raw, _ := claims["email"].(string)
	return strings.TrimSpace(raw)
}
