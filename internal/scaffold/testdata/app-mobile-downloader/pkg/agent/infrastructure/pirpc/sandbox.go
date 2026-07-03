package pirpc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SandboxRoot es la raíz donde se crean los workspaces por sesión para el
// agente pi. Está deliberadamente bajo tmp/ para que el watcher de air
// (que excluye "tmp") ignore lo que el agente escriba acá y no dispare
// rebuilds del servidor mientras una conversación está en curso.
const SandboxRoot = "tmp/agent-work"

// resolveCWD toma el CWD que el caller configuró para la sesión y devuelve
// el directorio efectivo donde se va a lanzar pi.
//
//   - Si el caller pasó un path explícito y no vacío, se respeta tal cual.
//     De este modo podemos operar sobre un submodule, worktree, o copia
//     específica cuando lo necesitemos.
//   - Si el CWD quedó vacío o es "." (default histórico), se redirige a
//     tmp/agent-work/<sessionID>/ — un sandbox por sesión excluido del
//     watcher de air. Esto evita que el agente modifique por accidente
//     archivos del repo (que dispararían recompilación y romperían la
//     sesión en curso).
//
// El directorio se crea con MkdirAll si no existe. Errores se propagan para
// que el runner falle rápido y el Manager muestre el error al usuario
// (en lugar de arrancar un pi huérfano).
func resolveCWD(specCWD, sessionID string) (string, error) {
	trimmed := strings.TrimSpace(specCWD)
	if trimmed != "" && trimmed != "." && trimmed != "./" {
		return trimmed, nil
	}

	safe := sanitizeSessionID(sessionID)
	if safe == "" {
		return "", fmt.Errorf("pirpc: empty session id for sandbox")
	}

	dir := filepath.Join(SandboxRoot, safe)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("pirpc: creating sandbox %q: %w", dir, err)
	}

	// Devolvemos el path absoluto para que los mensajes de log sean
	// inequívocos y pi no dependa del cwd del proceso Go.
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("pirpc: abs of %q: %w", dir, err)
	}
	return abs, nil
}

// sanitizeSessionID reduce un id de sesión arbitrario a un nombre seguro
// para usar como subcarpeta. Sólo letras, dígitos, guiones, guiones bajos
// y puntos. Cualquier otro carácter se reemplaza por '-'.
func sanitizeSessionID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		return ""
	}
	// Si quedó relleno sólo con separadores, no es útil como nombre de subdir.
	hasAlnum := false
	for _, r := range out {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			hasAlnum = true
			break
		}
	}
	if !hasAlnum {
		return ""
	}
	return out
}
