package pirpc

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentapp "lastmile-agents/internal/agent/application"
)

// honchoExtensionName es el nombre del directorio/archivo de la
// extensión Honcho en .pi/extensions/. Se filtra del spawn
// cuando el host enruta memoria vía MemoryProvider
// (DisableNativeHonchoTools=true) para evitar doble consumo de
// tokens.
//
// Hoy este repo no tiene tal extensión (verificado
// 2026-07-19: .pi/extensions/ contiene sólo provider.ts y
// smoke.ts), así que el filtro es forward-compat. La constante
// queda para que el día que alguien agregue .pi/extensions/honcho
// o .pi/extensions/honcho.ts, el wiring ya esté hecho.
const honchoExtensionName = "honcho"

func startProcess(ctx context.Context, binary string, spec agentapp.StartSpec) (*exec.Cmd, io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	args := []string{"--mode", "rpc"}
	if sessionFile := strings.TrimSpace(spec.SessionFile); sessionFile != "" {
		if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
			return nil, nil, nil, nil, err
		}
		args = append(args, "--session", sessionFile)
	}
	if title := strings.TrimSpace(spec.Title); title != "" {
		args = append(args, "--name", title)
	}
	if model := strings.TrimSpace(spec.Model); model != "" {
		args = append(args, "--model", model)
	}

	cwd, err := resolveCWD(spec.CWD, spec.SessionID, spec.AgentID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, extPath := range discoverProjectExtensions(cwd, spec.AgentID) {
		if spec.DisableNativeHonchoTools && isHonchoExtensionPath(extPath) {
			slog.Info("pirpc: skipping honcho extension due to DisableNativeHonchoTools",
				"session_id", spec.SessionID,
				"path", extPath,
			)
			continue
		}
		args = append(args, "-e", extPath)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	if len(args) > 2 {
		slog.Info("pirpc: starting pi",
			"session_id", spec.SessionID,
			"cwd", cwd,
			"args", args,
		)
	}
	cmd.Dir = cwd
	if cwd != strings.TrimSpace(spec.CWD) {
		slog.Info("pirpc: pi ejecutado en sandbox",
			"session_id", spec.SessionID,
			"cwd", cwd,
		)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, nil, err
	}
	return cmd, stdin, stdout, stderr, nil
}

func discoverProjectExtensions(cwd, agentID string) []string {
	if strings.TrimSpace(agentID) == "" {
		agentID = agentapp.DefaultAgentID()
	}
	roots := []string{filepath.Join(cwd, PiConfigDir, "extensions")}
	if rootCWD, err := os.Getwd(); err == nil {
		// Fallback: extensions del workspace del agente en la raíz
		// del repo. Se usa cuando el cwd del proceso no coincide
		// con la raíz del repo (ej. tests con chdir a tmpdir) y
		// queremos encontrar las extensions declaradas en el
		// workspace del agente.
		fallback := filepath.Join(rootCWD, AgentsRoot, agentID, PiConfigDir, "extensions")
		if !samePath(fallback, roots[0]) {
			roots = append(roots, fallback)
		}
	}

	// Recolecta candidatos primero, dedupea por path (vía `seen`)
	// y por contenido (vía `seenContent`). El dedup por contenido
	// es clave cuando el worktree del agente y el repo principal
	// tienen `.pi/extensions/` idéntico (mismo provider.ts en
	// ambos directorios): antes pasábamos el mismo archivo como
	// 2 `-e` flags distintos, lo que hacía que pi ejecutara el
	// handler `session_start` 2 veces por spawn (doble registro
	// de provider, doble setModel, posibles side effects
	// duplicados). Ver pirpc/process_test.go:TestDiscoverProjectExtensions_DedupByContent.
	seen := map[string]struct{}{}
	seenContent := map[string]string{} // md5 hex -> first path with that content
	paths := make([]string, 0, 8)

	add := func(fullPath string) {
		if _, ok := seen[fullPath]; ok {
			return
		}
		seen[fullPath] = struct{}{}
		// Si podemos leer el archivo, dedupeamos por contenido.
		// Si no se puede leer, dejamos pasar (mejor duplicar
		// un archivo ilegible que perder extensiones).
		if data, err := os.ReadFile(fullPath); err == nil {
			sum := md5.Sum(data)
			hash := hex.EncodeToString(sum[:])
			if first, dup := seenContent[hash]; dup {
				slog.Info("pirpc: skipping duplicate extension by content",
					"path", fullPath,
					"duplicate_of", first,
					"md5", hash,
				)
				return
			}
			seenContent[hash] = fullPath
		}
		paths = append(paths, fullPath)
	}

	for _, extDir := range roots {
		entries, err := os.ReadDir(extDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			fullPath := filepath.Join(extDir, name)
			if entry.IsDir() {
				for _, candidate := range []string{filepath.Join(fullPath, "index.ts"), filepath.Join(fullPath, "index.js")} {
					if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
						add(candidate)
						break
					}
				}
				continue
			}
			if strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js") {
				add(fullPath)
			}
		}
	}
	return paths
}

func samePath(a, b string) bool {
	a = filepath.Clean(strings.TrimSpace(a))
	b = filepath.Clean(strings.TrimSpace(b))
	if a == b {
		return true
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA == nil && errB == nil {
		return absA == absB
	}
	return false
}

// isHonchoExtensionPath devuelve true si el path apunta a la
// extensión Honcho dentro de .pi/extensions/. Matchea:
//   - archivo suelto:  .pi/extensions/honcho.ts
//   - directorio:      .pi/extensions/honcho/index.ts
//
// Usado por el filtro de DisableNativeHonchoTools en startProcess.
func isHonchoExtensionPath(p string) bool {
	if p == "" {
		return false
	}
	// Caso 1: el basename es la extensión (con o sin extensión).
	base := filepath.Base(p)
	if name := strings.TrimSuffix(base, filepath.Ext(base)); name == honchoExtensionName {
		return true
	}
	// Caso 2: el parent dir es la extensión (formato directorio/index.ts).
	parent := filepath.Base(filepath.Dir(p))
	return parent == honchoExtensionName
}
