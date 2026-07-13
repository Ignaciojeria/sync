//go:build linux

package application

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// KillProcessesInWorkspace termina cualquier proceso cuyo CWD (resuelto
// con symlinks) esté dentro de workspacePath. Best-effort: errores
// individuales se loguean y la operación continúa con el siguiente PID.
//
// Garantías de seguridad:
//   - Nunca mata el PID actual ni el PID 1.
//   - Solo mata procesos cuyo UID coincida con el del caller (evita
//     tocar procesos de otros usuarios en sistemas multi-tenant).
//   - Si workspacePath no existe, devuelve (0, nil) sin error.
//
// Comportamiento:
//   - Envía SIGTERM, espera hasta 500 ms, y a los sobrevivientes les
//     envía SIGKILL. Esto da chance a procesos bien portados (air, vite,
//     next dev) de cerrar sus file descriptors limpiamente.
//   - Devuelve la cantidad de procesos matados (SIGKILL cuenta, SIGTERM
//     exitoso también).
//
// Es Linux-only porque lee /proc/<pid>/{cwd,status}. En otros OS existe
// un stub no-op en process_cleanup_other.go.
func KillProcessesInWorkspace(ctx context.Context, workspacePath string) (int, error) {
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return 0, nil
	}

	// filepath.Abs + EvalSymlinks para que la comparación contra los
	// targets de /proc/<pid>/cwd sea robusta frente a symlinks y rutas
	// relativas.
	absTarget, err := filepath.Abs(workspacePath)
	if err != nil {
		return 0, fmt.Errorf("abs workspace: %w", err)
	}
	resolvedTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("eval symlinks %q: %w", absTarget, err)
	}
	resolvedTarget = filepath.Clean(resolvedTarget) + string(os.PathSeparator)

	myPID := os.Getpid()
	myUID := os.Getuid()

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, fmt.Errorf("read /proc: %w", err)
	}

	type target struct {
		pid  int
		uid  int
		comm string
	}
	var targets []target

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // no es un PID
		}
		if pid == 1 || pid == myPID {
			continue
		}

		// Filtrar por UID: leer /proc/<pid>/status es cheap y nos
		// protege de mandar señales a procesos de otros users.
		uid, comm, err := readProcUIDAndComm(pid)
		if err != nil {
			continue // permiso denegado o proceso que se está yendo
		}
		if uid != myUID {
			continue
		}

		// /proc/<pid>/cwd es un symlink al CWD real. Readlink lo
		// resuelve sin tener que entrar al directorio.
		cwd, err := os.Readlink(filepath.Join("/proc", entry.Name(), "cwd"))
		if err != nil {
			continue
		}
		// Resolver symlinks del cwd reportado; puede tener .. o
		// links intermedios.
		resolvedCWD, err := filepath.EvalSymlinks(cwd)
		if err != nil {
			continue
		}
		resolvedCWD = filepath.Clean(resolvedCWD)
		if !strings.HasPrefix(resolvedCWD+string(os.PathSeparator), resolvedTarget) &&
			resolvedCWD != strings.TrimSuffix(resolvedTarget, string(os.PathSeparator)) {
			continue
		}

		targets = append(targets, target{pid: pid, uid: uid, comm: comm})
	}

	killed := 0
	for _, t := range targets {
		if err := syscall.Kill(t.pid, syscall.SIGTERM); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				continue // ya murió
			}
			continue // EPERM u otro: lo salteamos, best-effort
		}
		if waitProcExit(ctx, t.pid, 500*time.Millisecond) {
			killed++
			continue
		}
		// No se fue con SIGTERM, escalar a SIGKILL.
		if err := syscall.Kill(t.pid, syscall.SIGKILL); err == nil {
			killed++
		}
	}
	return killed, nil
}

// readProcUIDAndComm lee Uid y Comm desde /proc/<pid>/status. Es más
// barato que hacer un syscall y no requiere permisos especiales más allá
// de los que ya tiene el owner del proceso para su propio /proc.
func readProcUIDAndComm(pid int) (int, string, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, "", err
	}
	var uid int
	var comm string
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "Uid:"):
			// Formato: "Uid:\treal\teffective\tsavedfs\tsavedset"
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				uid, _ = strconv.Atoi(fields[1])
			}
		case strings.HasPrefix(line, "Comm:"):
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				comm = fields[1]
			}
		}
	}
	if uid == 0 && comm == "" {
		return 0, "", errors.New("proc status vacío")
	}
	return uid, comm, nil
}

// waitProcExit hace polling de /proc/<pid>/exist durante el timeout.
// True si el proceso desapareció (sea por nuestra señal o por otra
// razón concurrente).
func waitProcExit(ctx context.Context, pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	tick := 25 * time.Millisecond
	for {
		if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return true
			}
			return false
		}
		if ctx.Err() != nil {
			return false
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(tick)
		if tick < 100*time.Millisecond {
			tick *= 2
		}
	}
}