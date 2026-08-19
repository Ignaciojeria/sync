package fs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitinittest5/internal/backlog/application"
)

// Store es la implementación filesystem-backed de application.Store.
//
// Es seguro para uso concurrente: todas las mutaciones pasan por
// flocks por columna, y la lectura usa un cache indexado por mtime
// del directorio raíz que se invalida cuando algún subdir cambia
// (SPEC §8).
type Store struct {
	root string

	mu         sync.RWMutex
	cache      []application.Card     // resultado del último List
	cacheAt    time.Time              // mtime del root al cachear
	cacheValid bool
}

// NewStore crea un Store sobre root. Crea el directorio raíz y los
// cuatro subdirectorios de columna si no existen (idempotente).
func NewStore(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("backlog fs: root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}
	for _, s := range application.ColumnOrder {
		if err := os.MkdirAll(filepath.Join(abs, string(s)), 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", s, err)
		}
	}
	return &Store{root: abs}, nil
}

// Root devuelve la ruta absoluta al bundle.
func (s *Store) Root() string { return s.root }

// List devuelve todas las tarjetas del bundle. Cachea por mtime del
// root: si nada cambió desde el último List, devuelve el cache.
func (s *Store) List(ctx context.Context) ([]application.Card, error) {
	if cached, ok := s.cachedList(); ok {
		return cached, nil
	}

	var out []application.Card
	for _, status := range application.ColumnOrder {
		cards, err := s.scanDir(s.dirFor(status))
		if err != nil {
			return nil, err
		}
		out = append(out, cards...)
	}

	s.mu.Lock()
	s.cache = out
	s.cacheAt = s.rootMtime()
	s.cacheValid = true
	s.mu.Unlock()

	// Devolver copia para que el caller no muté nuestro cache.
	cpy := make([]application.Card, len(out))
	copy(cpy, out)
	return cpy, nil
}

// Board es la vista agregada del bundle.
func (s *Store) Board(ctx context.Context) (application.Board, []application.InvalidCard, error) {
	cards, err := s.List(ctx)
	if err != nil {
		return application.Board{}, nil, err
	}
	board, invalids := application.ToBoard(cards)
	return board, invalids, nil
}

// Get devuelve una tarjeta por slug.
func (s *Store) Get(ctx context.Context, slug string) (application.Card, error) {
	if slug == "" {
		return application.Card{}, fmt.Errorf("%w: slug is required", application.ErrInvalidInput)
	}
	cards, err := s.List(ctx)
	if err != nil {
		return application.Card{}, err
	}
	for _, c := range cards {
		if c.Slug == slug {
			return c, nil
		}
	}
	return application.Card{}, application.ErrNotFound
}

// Create escribe una nueva tarjeta en dir, derivando el slug del
// título. Si hay colisión, suffija.
//
// NO se expone en la UI web (los humanos no crean tarjetas); solo
// lo usa el backlog-cli (en tmp/backlog-cli) cuando el agente siembra
// tareas. El camino alternativo es que el agente escriba el .md
// directamente.
func (s *Store) Create(ctx context.Context, dir application.Status, card application.Card) (application.Card, error) {
	if !dir.Valid() {
		return application.Card{}, fmt.Errorf("%w: status %q", application.ErrInvalidInput, dir)
	}
	targetDir := s.dirFor(dir)

	// Coleccionar slugs en uso bajo flock para resolver colisiones
	// atómicamente.
	var usedSlugs map[string]bool
	if err := WithLock(targetDir, func() error {
		usedSlugs = s.collectSlugs(targetDir)
		return nil
	}); err != nil {
		return application.Card{}, err
	}

	card.Type = application.DefaultType
	if card.Slug == "" {
		card.Slug = application.Slugify(card.Title, usedSlugs)
	}
	card.Status = dir
	if card.Path == "" {
		card.Path = filepath.Join(targetDir, card.Slug+".md")
	}

	// Validar antes de escribir.
	if err := card.Validate(); err != nil {
		return application.Card{}, fmt.Errorf("%w: %s", application.ErrInvalidInput, err.Error())
	}

	if err := WithLock(targetDir, func() error {
		return s.writeCardAtomic(card, nil)
	}); err != nil {
		return application.Card{}, err
	}
	s.invalidate()
	return card, nil
}

// Update reescribe una tarjeta existente. Si el slug derivado del
// nuevo title difiere del actual, hace rename atómico al nuevo path.
func (s *Store) Update(ctx context.Context, slug string, card application.Card) (application.Card, error) {
	if slug == "" {
		return application.Card{}, fmt.Errorf("%w: slug is required", application.ErrInvalidInput)
	}

	cur, err := s.Get(ctx, slug)
	if err != nil {
		return application.Card{}, err
	}

	// Merge con la card actual: campos que el caller no especificó
	// caen al valor persistido. Si el caller tampoco envió source,
	// caemos al default "user".
	if card.Source == "" {
		card.Source = cur.Source
		if card.Source == "" {
			card.Source = application.DefaultSource
		}
	}
	if card.AgentSession == "" {
		card.AgentSession = cur.AgentSession
	}
	if len(card.Tags) == 0 {
		card.Tags = cur.Tags
	}
	if card.Description == "" {
		card.Description = cur.Description
	}
	if card.Body == "" {
		card.Body = cur.Body
	}
	if card.Timestamp == "" {
		card.Timestamp = application.NowRFC3339()
	}
	if card.Priority == "" {
		card.Priority = cur.Priority
		if card.Priority == "" {
			card.Priority = application.DefaultPriority
		}
	}

	// Recolectar slugs usados para resolver posible rename.
	// Excluimos cur.Slug del set: el slot actual es "nuestro" y
	// debe estar disponible aunque el filename matchee el slugify
	// del nuevo title (caso típico: update sin cambiar el title).
	usedSlugs := s.collectAllSlugs()
	delete(usedSlugs, cur.Slug)
	newSlug := application.Slugify(card.Title, usedSlugs)
	willRename := newSlug != cur.Slug

	card.Type = application.DefaultType
	card.Slug = newSlug
	card.Status = cur.Status
	card.Path = filepath.Join(s.dirFor(cur.Status), newSlug+".md")

	if err := card.Validate(); err != nil {
		return application.Card{}, fmt.Errorf("%w: %s", application.ErrInvalidInput, err.Error())
	}

	// Lockear origen (y destino si hay rename) en orden lexicográfico
	// para evitar deadlock con operaciones cross-column concurrentes.
	srcDir := s.dirFor(cur.Status)
	var originalFM map[string]any
	if err := WithLocks([]string{srcDir}, func() error {
		// Releer el archivo bajo lock para preservar el FM original.
		raw, err := os.ReadFile(cur.Path)
		if err != nil {
			return fmt.Errorf("read original: %w", err)
		}
		res, err := application.ParseCardFile(cur.Path, raw)
		if err != nil {
			return fmt.Errorf("re-parse: %w", err)
		}
		originalFM = res.Frontmatter

		if willRename {
			if err := os.Rename(cur.Path, card.Path); err != nil {
				return fmt.Errorf("rename: %w", err)
			}
		}
		return s.writeCardAtomic(card, originalFM)
	}); err != nil {
		return application.Card{}, err
	}
	s.invalidate()
	return card, nil
}

// Move cambia la columna de una tarjeta. No-op si ya está ahí.
func (s *Store) Move(ctx context.Context, slug string, to application.Status) (application.Card, error) {
	if !to.Valid() {
		return application.Card{}, fmt.Errorf("%w: status %q", application.ErrInvalidInput, to)
	}
	cur, err := s.Get(ctx, slug)
	if err != nil {
		return application.Card{}, err
	}
	if cur.Status == to {
		return cur, nil
	}

	srcDir := s.dirFor(cur.Status)
	dstDir := s.dirFor(to)

	var updated application.Card
	if err := WithLocks([]string{srcDir, dstDir}, func() error {
		raw, err := os.ReadFile(cur.Path)
		if err != nil {
			return fmt.Errorf("read original: %w", err)
		}
		res, err := application.ParseCardFile(cur.Path, raw)
		if err != nil {
			return fmt.Errorf("re-parse: %w", err)
		}
		base := res.Card
		base.Status = to
		base.Path = filepath.Join(dstDir, base.Slug+".md")

		usedSlugs := s.collectSlugs(dstDir)
		if usedSlugs[base.Slug] {
			// Renombrar para evitar pisar.
			base.Slug = application.Slugify(base.Title, usedSlugs)
			base.Path = filepath.Join(dstDir, base.Slug+".md")
		}
		base.Timestamp = application.NowRFC3339()

		if err := os.Rename(filepath.Join(srcDir, res.Card.Slug+".md"), base.Path); err != nil {
			return fmt.Errorf("rename: %w", err)
		}
		if err := s.writeCardAtomic(base, res.Frontmatter); err != nil {
			return err
		}
		updated = base
		return nil
	}); err != nil {
		return application.Card{}, err
	}
	s.invalidate()
	return updated, nil
}

// SetPriority cambia solo la prioridad.
func (s *Store) SetPriority(ctx context.Context, slug string, p application.Priority) (application.Card, error) {
	cur, err := s.Get(ctx, slug)
	if err != nil {
		return application.Card{}, err
	}
	cur.Priority = p
	cur.Timestamp = application.NowRFC3339()
	if err := WithLock(s.dirFor(cur.Status), func() error {
		raw, rerr := os.ReadFile(cur.Path)
		if rerr != nil {
			return rerr
		}
		res, perr := application.ParseCardFile(cur.Path, raw)
		if perr != nil {
			return perr
		}
		res.Card.Priority = p
		res.Card.Timestamp = cur.Timestamp
		return s.writeCardAtomic(res.Card, res.Frontmatter)
	}); err != nil {
		return application.Card{}, err
	}
	s.invalidate()
	return cur, nil
}

// Delete elimina una tarjeta por slug.
func (s *Store) Delete(ctx context.Context, slug string) error {
	cur, err := s.Get(ctx, slug)
	if err != nil {
		return err
	}
	if err := WithLock(s.dirFor(cur.Status), func() error {
		if err := os.Remove(cur.Path); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// --- internals ---

func (s *Store) dirFor(status application.Status) string {
	return filepath.Join(s.root, string(status))
}

// scanDir lee todos los .md de dir y los parsea. Los archivos que
// fallan el parseo se loguean y se excluyen (SPEC §7).
func (s *Store) scanDir(dir string) ([]application.Card, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("readdir %s: %w", dir, err)
	}
	out := make([]application.Card, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == LockFileName || name == ".keep" || name == "index.md" || name == "log.md" {
			continue
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(dir, name)
		raw, err := os.ReadFile(path)
		if err != nil {
			log.Printf("backlog: read %s: %v", path, err)
			continue
		}
		res, err := application.ParseCardFile(path, raw)
		if err != nil {
			log.Printf("backlog: invalid card at %s: %v", path, err)
			continue
		}
		out = append(out, res.Card)
	}
	return out, nil
}

// collectSlugs devuelve los slugs ya usados en dir.
func (s *Store) collectSlugs(dir string) map[string]bool {
	cards, _ := s.scanDir(dir)
	out := make(map[string]bool, len(cards))
	for _, c := range cards {
		out[c.Slug] = true
	}
	return out
}

// collectAllSlugs devuelve los slugs en uso en todas las columnas.
func (s *Store) collectAllSlugs() map[string]bool {
	out := map[string]bool{}
	for _, status := range application.ColumnOrder {
		for s, used := range s.collectSlugs(s.dirFor(status)) {
			out[s] = used
		}
	}
	return out
}

// writeCardAtomic serializa card y la escribe atómicamente
// (temp file + rename).
func (s *Store) writeCardAtomic(card application.Card, originalFM map[string]any) error {
	data, err := application.WriteCardFile(card, originalFM)
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}
	tmp := card.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, card.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// invalidate descarta el cache.
func (s *Store) invalidate() {
	s.mu.Lock()
	s.cacheValid = false
	s.mu.Unlock()
}

// cachedList devuelve el cache si está fresco.
func (s *Store) cachedList() ([]application.Card, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.cacheValid {
		return nil, false
	}
	if !s.cacheAt.Equal(s.rootMtime()) {
		return nil, false
	}
	cpy := make([]application.Card, len(s.cache))
	copy(cpy, s.cache)
	return cpy, true
}

// rootMtime devuelve el mtime más reciente entre root y sus
// subdirectorios. Se usa como señal de invalidación del cache.
func (s *Store) rootMtime() time.Time {
	var latest time.Time
	_ = filepath.Walk(s.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
		return nil
	})
	return latest
}
