package pirpc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	agentapp "lastmile-agents/internal/agent/application"
)

// PiConfigDir es el nombre de la carpeta de configuración de pi que se
// siembra en cada sandbox de sesión. Vive dentro del workspace del
// agente en agents/<agentID>/.pi/ (relativa al CWD del proceso
// agent-worker) y se copia entera al sandbox para que la sesión
// conserve extensiones, provider y demás configuración.
const PiConfigDir = ".pi"

// AgentsRoot es la carpeta donde viven los workspaces de cada agente
// del repo. Cada subcarpeta contiene un .pi/ y un AGENTS.md que el
// runner siembra en el sandbox de la sesión. Default: agents/.
// Si en el futuro se quiere mover a otro path, esta constante es el
// único punto a tocar.
const AgentsRoot = "agents"

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
//   - Si el CWD apunta a la RAÍZ del repo (donde antes vivía .pi/ raíz,
//     hoy vive en agents/<agentID>/.pi/) y existe el workspace del
//     agente, redirigimos al workspace del agente. Sin esto, pi corre
//     desde la raíz sin encontrar su .pi/ y queda con catálogo de
//     modelos incompleto + sin API key. El agente puede `cd ..` para
//     acceder al resto del repo.
//
// agentID identifica qué agente corre la sesión (ej. "develop"). Si
// viene vacío se resuelve al default del registry (DefaultAgentID).
// Se usa para sembrar el .pi/ y AGENTS.md desde agents/<agentID>/.
// Esto sienta las bases para multi-agente: cada agente tiene su
// propio workspace en el repo.
//
// El directorio se crea con MkdirAll si no existe. Errores se propagan para
// que el runner falle rápido y el Manager muestre el error al usuario
// (en lugar de arrancar un pi huérfano).
func resolveCWD(specCWD, sessionID, agentID string) (string, error) {
	trimmed := strings.TrimSpace(specCWD)

	if strings.TrimSpace(agentID) == "" {
		agentID = agentapp.DefaultAgentID()
	}

	// ponytail: si el caller pasó un CWD explícito y ese CWD
	// contiene el workspace del agente (agents/<id>/ con .pi/
	// adentro), redirigimos al workspace del agente. Esto cubre
	// el caso típico del worktree de sesión: CWD = raíz del repo
	// (donde antes vivía .pi/), .pi/ ahora vive en
	// agents/<agentID>/.pi/ dentro de ese mismo repo. Sin la
	// redirección, pi arranca con config vacío (catálogo de
	// modelos incompleto, API key no encontrada).
	//
	// Si no se cumple ninguna de las dos condiciones (no hay
	// workspace del agente en el CWD, o el CWD está vacío),
	// caemos al comportamiento original (sandbox).
	if trimmed != "" && trimmed != "." && trimmed != "./" {
		if redirected, err := maybeRedirectToAgentWorkspace(trimmed, agentID); err != nil {
			return "", err
		} else if redirected != "" {
			return redirected, nil
		}
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

	// Sembramos la configuración del agente (.pi entero) desde
	// agents/<agentID>/.pi/ para que la sesión conserve extensiones,
	// provider y demás setup del agente. Es idempotente: si el sandbox
	// ya tiene un .pi (sesión en curso o previamente sembrada) no se
	// pisa.
	if err := seedPiConfig(dir, agentID); err != nil {
		return "", fmt.Errorf("pirpc: seeding %s for agent %q: %w", PiConfigDir, agentID, err)
	}
	// Copiamos el AGENTS.md del workspace del agente al sandbox para
	// que pi lo consuma como system prompt del agente. Idempotente:
	// no pisa si ya está presente. El AGENTS.md raíz del repo NO se
	// siembra acá — es la fuente de reglas para humanos que abren el
	// repo, no para el agente embebido.
	if err := seedAgentsDoc(dir, agentID); err != nil {
		return "", fmt.Errorf("pirpc: seeding AGENTS.md for agent %q: %w", agentID, err)
	}

	// Devolvemos el path absoluto para que los mensajes de log sean
	// inequívocos y pi no dependa del cwd del proceso Go.
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("pirpc: abs of %q: %w", dir, err)
	}
	return abs, nil
}

// maybeRedirectToAgentWorkspace chequea si cwd es un repo con un
// workspace de agente (agents/<id>/) y devuelve la ruta absoluta al
// workspace del agente para que pi corra desde ahí y encuentre su
// .pi/. Si no hay workspace del agente en ese cwd, devuelve "" y el
// caller sigue con el flujo normal (CWD tal cual o sandbox).
//
// Esto evita el bug del merge multi-agente donde .pi/ se movió a
// agents/<id>/.pi/: sesiones con CWD = raíz del repo quedaban con
// pi corriendo sin config, catálogo de modelos incompleto y sin
// API key.
func maybeRedirectToAgentWorkspace(cwd, agentID string) (string, error) {
	if strings.TrimSpace(agentID) == "" {
		return "", nil
	}
	agentWS := filepath.Join(cwd, AgentsRoot, agentID)
	info, err := os.Stat(agentWS)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil // no hay workspace del agente; no redirigir
		}
		return "", err
	}
	if !info.IsDir() {
		return "", nil
	}
	// Confirmamos que tiene .pi/ — sin .pi/ no hay nada que ganar
	// cambiando de directorio.
	piDir := filepath.Join(agentWS, PiConfigDir)
	if _, err := os.Stat(piDir); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	abs, err := filepath.Abs(agentWS)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// agentWorkspacePath devuelve la ruta absoluta al workspace del agente
// en el repo (agents/<agentID>/). Usado como source para sembrar
// .pi y AGENTS.md en el sandbox.
func agentWorkspacePath(agentID string) string {
	return filepath.Join(AgentsRoot, strings.TrimSpace(agentID))
}

// seedPiConfig copia la carpeta .pi del workspace del agente
// (agents/<agentID>/.pi) dentro del sandbox (destDir). Es idempotente:
// si el destino ya tiene un .pi no hace nada, evitando pisar la
// configuración de una sesión en curso.
//
// El source se resuelve relativo al CWD del proceso agent-worker (la raíz
// del repo). Si el workspace del agente no tiene .pi, no es un error:
// simplemente no hay nada que sembrar y la sesión arranca con la config
// por defecto de pi.
func seedPiConfig(destDir, agentID string) error {
	src := filepath.Join(agentWorkspacePath(agentID), PiConfigDir)
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no hay .pi en el workspace; nada que sembrar
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s no es un directorio", src)
	}

	dst := filepath.Join(destDir, PiConfigDir)
	if _, err := os.Stat(dst); err == nil {
		return nil // ya sembrado; no pisar
	} else if !os.IsNotExist(err) {
		return err
	}

	return copyDir(src, dst)
}

// seedAgentsDoc copia el AGENTS.md del workspace del agente
// (agents/<agentID>/AGENTS.md) al sandbox (destDir/AGENTS.md). Es
// idempotente: no pisa si ya está presente en el sandbox (la sesión
// podría haberlo editado o anotado algo).
//
// El AGENTS.md raíz del repo NO se copia — es la fuente de reglas
// para humanos/IA que abren el repo desde su editor, no para el
// agente embebido. El agente consume únicamente el AGENTS.md del
// workspace de su agentID.
//
// Si el workspace del agente no tiene AGENTS.md, no es un error:
// simplemente no se siembra nada (el sandbox queda sin ese archivo
// y pi operará sin reglas específicas del agente en este caso).
func seedAgentsDoc(destDir, agentID string) error {
	src := filepath.Join(agentWorkspacePath(agentID), "AGENTS.md")
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no hay AGENTS.md en el workspace; nada que sembrar
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s no es un archivo regular", src)
	}

	dst := filepath.Join(destDir, "AGENTS.md")
	if _, err := os.Stat(dst); err == nil {
		return nil // ya sembrado; no pisar
	} else if !os.IsNotExist(err) {
		return err
	}

	return copyFile(src, dst, info.Mode().Perm())
}

// copyDir copia recursivamente el árbol de src a dst preservando permisos.
func copyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		switch {
		case entry.IsDir():
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		case entry.Type()&os.ModeSymlink != 0:
			// Resolvemos el symlink y copiamos el target real para que el
			// sandbox quede autocontenido.
			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(srcPath), target)
			}
			ti, err := os.Stat(target)
			if err != nil {
				return err
			}
			if ti.IsDir() {
				if err := copyDir(target, dstPath); err != nil {
					return err
				}
			} else if err := copyFile(target, dstPath, ti.Mode().Perm()); err != nil {
				return err
			}
		default:
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if err := copyFile(srcPath, dstPath, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copia un archivo regular de src a dst con el modo indicado.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
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
