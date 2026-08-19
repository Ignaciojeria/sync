// Package fs implementa application.Store sobre el sistema de
// archivos. Es el único paquete del módulo backlog que toca el disco.
//
// Concurrencia: las operaciones que mutan el FS toman un flock
// (LOCK_EX) sobre un archivo .lock dentro del directorio
// correspondiente. Cross-column (Move, Update con rename) toma dos
// locks en orden lexicográfico para evitar deadlock.
package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// LockFileName es el nombre del archivo usado como punto de flock
// dentro de cada columna. Está oculto (.) para no aparecer en listados
// del bundle OKF.
const LockFileName = ".lock"

// DirLock representa un flock activo sobre el directorio dir.
type DirLock struct {
	dir string
	f   *os.File
}

// LockDir toma un flock exclusivo sobre dir. Si dir no existe, NO lo
// crea (la creación de directorios es responsabilidad del Store en
// arranque).
//
// El caller debe invocar Unlock() al terminar. El método es seguro de
// llamar múltiples veces.
func LockDir(dir string) (*DirLock, error) {
	lockPath := filepath.Join(dir, LockFileName)
	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file %s: %w", lockPath, err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock %s: %w", lockPath, err)
	}
	return &DirLock{dir: dir, f: f}, nil
}

// Unlock libera el flock y cierra el archivo.
func (l *DirLock) Unlock() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	cerr := l.f.Close()
	l.f = nil
	if err != nil {
		return err
	}
	return cerr
}

// WithLock toma el flock sobre dir, ejecuta fn, y libera. Conveniente
// para operaciones one-shot. Si fn devuelve error, el lock igual se
// libera antes de propagarlo.
func WithLock(dir string, fn func() error) error {
	l, err := LockDir(dir)
	if err != nil {
		return err
	}
	defer func() {
		_ = l.Unlock()
	}()
	return fn()
}

// WithLocks toma flocks exclusivos sobre cada dir en dirs, en orden
// lexicográfico ascendente, ejecuta fn, y libera en orden inverso.
// Esto garantiza que dos operaciones cross-column concurrentes nunca
// tomen locks en órdenes opuestos (deadlock).
func WithLocks(dirs []string, fn func() error) error {
	uniq := make([]string, 0, len(dirs))
	seen := map[string]bool{}
	for _, d := range dirs {
		if !seen[d] {
			uniq = append(uniq, d)
			seen[d] = true
		}
	}
	sort.Strings(uniq)

	locks := make([]*DirLock, 0, len(uniq))
	for _, d := range uniq {
		l, err := LockDir(d)
		if err != nil {
			// Liberar los ya tomados antes de propagar.
			for i := len(locks) - 1; i >= 0; i-- {
				_ = locks[i].Unlock()
			}
			return err
		}
		locks = append(locks, l)
	}
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			_ = locks[i].Unlock()
		}
	}()
	return fn()
}
