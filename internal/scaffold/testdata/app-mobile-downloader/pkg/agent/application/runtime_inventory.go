package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcessInfo describe un proceso del agente LLM en ejecución.
//
// Sólo listamos procesos cuyo `comm` (según /proc/<pid>/comm) es
// exactamente "pi". Eso coincide con el binario que el worker invoca
// vía pirpc.Runner, así que la lista refleja las runtimes del agente
// — ni Node genérico ni nada del worker mismo.
//
// Elapsed se devuelve formateado como "Hh Mm" (o "Mm Ss", "Ss" si
// es muy corto) para que sea legible sin obligar al cliente a
// convertir epoch/ticks. RSSKB viene directo de VmRSS en /proc, en
// kilobytes.
type ProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	CWD     string `json:"cwd"`
	Elapsed string `json:"elapsed"`
	RSSKB   int64  `json:"rssKb"`
	// Owner indica si el proceso parece asociado a una sesión
	// específica del agente (lo infiere del CWD que cae bajo
	// tmp/agent-work/<id>/). Vacío si no se puede determinar.
	Owner string `json:"owner,omitempty"`
}

// ListAgentRuntimes escanea /proc en busca de procesos cuyo comm sea
// "pi". Best-effort: si /proc no existe (no-Linux) devuelve error.
//
// El ctx no se usa para abortar el scan (que es rápido) pero queda en
// la firma para futura implementación de cancelación y para mantener
// consistencia con el resto del package.
func ListAgentRuntimes(ctx context.Context) ([]ProcessInfo, error) {
	_ = ctx
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}
	var out []ProcessInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue // "/proc/self", "/proc/thread-self", etc.
		}
		proc := "/proc/" + strconv.Itoa(pid)
		commBytes, err := os.ReadFile(filepath.Join(proc, "comm"))
		if err != nil {
			continue // proceso desapareció entre ReadDir y ahora
		}
		comm := strings.TrimRight(string(commBytes), "\n")
		if comm != "pi" {
			continue
		}
		rt, ok := readRuntimeInfo(proc, pid)
		if !ok {
			continue
		}
		out = append(out, rt)
	}
	return out, nil
}

// readRuntimeInfo lee /proc/<pid>/{cwd,stat,status} y arma el
// ProcessInfo. Devuelve ok=false si los datos críticos no se pueden
// leer (el proceso se murió en medio del scan).
func readRuntimeInfo(proc string, pid int) (ProcessInfo, bool) {
	r := ProcessInfo{PID: pid, Command: "pi"}

	if tgt, err := os.Readlink(filepath.Join(proc, "cwd")); err == nil {
		r.CWD = tgt
	}

	// /proc/<pid>/stat campo 22 = starttime en clock ticks desde boot.
	statBytes, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err == nil {
		fields := strings.Fields(string(statBytes))
		if len(fields) >= 22 {
			startJiffies, errP := strconv.ParseInt(fields[21], 10, 64)
			if errP == nil {
				uptimeBytes, errU := os.ReadFile("/proc/uptime")
				if errU == nil {
					upParts := strings.Fields(string(uptimeBytes))
					if len(upParts) >= 1 {
						uptimeSec, errF := strconv.ParseFloat(upParts[0], 64)
						if errF == nil {
							// USER_HZ en Linux estándar. Exótico rompería
							// el cálculo, pero el resto del inventario
							// sigue sirviendo.
							const clkTck = 100
							elapsed := int64(uptimeSec) - startJiffies/clkTck
							if elapsed < 0 {
								elapsed = 0
							}
							r.Elapsed = formatRuntimeElapsed(elapsed)
						}
					}
				}
			}
		}
	}

	if statusBytes, err := os.ReadFile(filepath.Join(proc, "status")); err == nil {
		for _, line := range strings.Split(string(statusBytes), "\n") {
			if strings.HasPrefix(line, "VmRSS:") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					n, errP := strconv.ParseInt(parts[1], 10, 64)
					if errP == nil {
						r.RSSKB = n
					}
				}
				break
			}
		}
	}

	// Inferimos owner desde el CWD: si está bajo tmp/agent-work/<id>/,
	// <id> es el session id.
	if cwd := r.CWD; strings.Contains(cwd, "/tmp/agent-work/") {
		const marker = "/tmp/agent-work/"
		idx := strings.Index(cwd, marker)
		if idx >= 0 {
			rest := cwd[idx+len(marker):]
			if slash := strings.Index(rest, "/"); slash >= 0 {
				rest = rest[:slash]
			}
			r.Owner = rest
		}
	}

	return r, true
}

func formatRuntimeElapsed(secs int64) string {
	if secs >= 3600 {
		return fmt.Sprintf("%dh%02dm", secs/3600, (secs%3600)/60)
	}
	if secs >= 60 {
		return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
	}
	return fmt.Sprintf("%ds", secs)
}
