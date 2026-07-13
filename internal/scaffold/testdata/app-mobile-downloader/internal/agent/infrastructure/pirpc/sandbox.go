package pirpc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PiConfigDir es el nombre de la carpeta de configuración de pi que se
// siembra en cada sandbox de sesión. Vive en la raíz del repo (relativa
// al CWD del proceso agent-worker) y se copia entera al sandbox para que
// la sesión conserve extensiones, provider y demás configuración.
const PiConfigDir = ".pi"

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

	// Sembramos la configuración de pi (.pi entero) desde la raíz del repo
	// para que la sesión conserve extensiones, provider y demás setup del
	// proyecto. Es idempotente: si el sandbox ya tiene un .pi (sesión en
	// curso o previamente sembrada) no se pisa.
	if err := seedPiConfig(dir); err != nil {
		return "", fmt.Errorf("pirpc: seeding %s into %q: %w", PiConfigDir, dir, err)
	}

	// Devolvemos el path absoluto para que los mensajes de log sean
	// inequívocos y pi no dependa del cwd del proceso Go.
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("pirpc: abs of %q: %w", dir, err)
	}
	return abs, nil
}

// seedPiConfig copia la carpeta .pi del repo (source) dentro del sandbox
// (destDir). Es idempotente: si el destino ya tiene un .pi no hace nada,
// evitando pisar la configuración de una sesión en curso.
//
// El source se resuelve relativo al CWD del proceso agent-worker (la raíz
// del repo). Si el repo no tiene .pi, no es un error: simplemente no hay
// nada que sembrar y la sesión arranca con la config por defecto de pi.
func seedPiConfig(destDir string) error {
	src := PiConfigDir
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no hay .pi en el repo; nada que sembrar
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
