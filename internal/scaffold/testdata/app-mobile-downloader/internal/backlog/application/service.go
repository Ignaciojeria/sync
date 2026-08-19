package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Store es el contrato de persistencia que la capa application
// consume. La implementación vive en infrastructure/fs. application
// no toca el disco directamente (Inversión de Dependencias).
//
// Concept: el ID externo de una tarjeta es el slug simple
// (último segmento del Concept ID, sin .md). El Store resuelve
// slug → path internamente porque mantiene un índice basado en
// mtime del directorio.
//
// Nota: la creación de tarjetas desde la UI web está deshabilitada:
// los humanos no crean tareas, solo el agente (vía CLI o
// escribiendo .md directamente al bundle). Create se mantiene
// en el contrato porque el backlog-cli (en tmp/backlog-cli) lo usa
// como brazo del agente.
type Store interface {
	// List devuelve TODAS las tarjetas del bundle (incluyendo las
	// que no son type=backlog/card, que serán filtradas por
	// ToBoard).
	List(ctx context.Context) ([]Card, error)

	// Get devuelve una tarjeta por slug. Devuelve ErrNotFound si no
	// existe o si el archivo es inválido.
	Get(ctx context.Context, slug string) (Card, error)

	// Create escribe una nueva tarjeta en dir, devolviendo la
	// tarjeta creada con Slug, Path y Timestamp asignados.
	// El slug se deriva del título; si hay colisión, el Store suffija.
	Create(ctx context.Context, dir Status, card Card) (Card, error)

	// Update reescribe el frontmatter de una tarjeta existente
	// identificada por slug, preservando keys desconocidas y el
	// body byte por byte. El slug puede cambiar si title cambió;
	// en ese caso el archivo se renombra.
	Update(ctx context.Context, slug string, card Card) (Card, error)

	// Move cambia la columna de una tarjeta, equivalente a mv al
	// directorio destino + update de status. No-op si ya está ahí.
	Move(ctx context.Context, slug string, to Status) (Card, error)

	// SetPriority cambia la prioridad sin tocar nada más.
	SetPriority(ctx context.Context, slug string, p Priority) (Card, error)

	// Delete elimina una tarjeta por slug. Idempotencia: borrar
	// algo que no existe devuelve ErrNotFound.
	Delete(ctx context.Context, slug string) error

	// Board devuelve la vista agregada lista para renderizar.
	// Equivale a List + ToBoard; convenience method para la UI.
	Board(ctx context.Context) (Board, []InvalidCard, error)
}

// Service es la fachada que consumen los handlers HTTP. Mantiene la
// misma forma que el Service anterior para minimizar el cambio en
// http/: List/Get/Create/Move/SetPriority/Update/Delete/Board.
type Service struct {
	store Store
	now   func() string
}

// NewService construye un Service sobre un Store. now puede ser nil
// para usar NowRFC3339; se inyecta para tests deterministas.
func NewService(store Store) *Service {
	return &Service{store: store, now: NowRFC3339}
}

// WithNow permite inyectar un clock para tests.
func (s *Service) WithNow(fn func() string) *Service {
	s.now = fn
	return s
}

// List devuelve todas las tarjetas del bundle en orden de board.
func (s *Service) List(ctx context.Context) ([]Card, error) {
	cards, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	board, _ := ToBoard(cards)
	out := make([]Card, 0, board.Count)
	for _, col := range board.Columns {
		out = append(out, col.Cards...)
	}
	return out, nil
}

// Get devuelve una tarjeta por slug. Devuelve ErrNotFound si no
// existe. Cards con type distinto a backlog/card o inválidas
// cuentan como not-found para el caller HTTP.
func (s *Service) Get(ctx context.Context, slug string) (Card, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Card{}, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	c, err := s.store.Get(ctx, slug)
	if err != nil {
		return Card{}, err
	}
	if c.Type != DefaultType {
		return Card{}, ErrNotFound
	}
	if err := c.Validate(); err != nil {
		return Card{}, ErrNotFound
	}
	return c, nil
}

// Create persiste una nueva tarjeta. Si status es vacío, cae en
// StatusBacklog. Si priority es inválida o vacía, cae en P3.
//
// Esta operación NO está expuesta en la UI web: los humanos no
// crean tarjetas. Solo la consume el backlog-cli (en tmp/backlog-cli)
// cuando el agente necesita sembrar una tarea (el camino alternativo
// es escribir el .md directamente al bundle).
func (s *Service) Create(ctx context.Context, title, description string, status Status, priority Priority) (Card, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" {
		return Card{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}
	if status == "" {
		status = StatusBacklog
	}
	if !status.Valid() {
		return Card{}, fmt.Errorf("%w: status %q", ErrInvalidInput, status)
	}
	if !priority.Valid() {
		priority = DefaultPriority
	}

	c := Card{
		Type:        DefaultType,
		Title:       title,
		Description: description,
		Status:      status,
		Priority:    priority,
		Source:      DefaultSource,
		Timestamp:   s.now(),
		Body:        "",
	}
	return s.store.Create(ctx, status, c)
}

// Update cambia título y descripción de una tarjeta. Si el título
// queda vacío, devuelve ErrInvalidInput.
func (s *Service) Update(ctx context.Context, slug, title, description string) (Card, error) {
	slug = strings.TrimSpace(slug)
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if slug == "" {
		return Card{}, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	if title == "" {
		return Card{}, fmt.Errorf("%w: title is required", ErrInvalidInput)
	}

	cur, err := s.Get(ctx, slug)
	if err != nil {
		return Card{}, err
	}
	cur.Title = title
	cur.Description = description
	cur.Timestamp = s.now()
	return s.store.Update(ctx, slug, cur)
}

// Move cambia la columna de una tarjeta. Saturación: si `to` es
// inválido, devuelve error.
func (s *Service) Move(ctx context.Context, slug string, to Status) (Card, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Card{}, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	if !to.Valid() {
		return Card{}, fmt.Errorf("%w: status %q", ErrInvalidInput, to)
	}
	return s.store.Move(ctx, slug, to)
}

// SetPriority cambia la prioridad sin tocar nada más. Saturación:
// si p está fuera de rango, se clampa a P0/P3 en vez de fallar.
func (s *Service) SetPriority(ctx context.Context, slug string, p Priority) (Card, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Card{}, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	if !p.Valid() {
		// Saturar en vez de fallar para que los botones ↑/↓ en los
		// extremos del control de prioridad no rompan la UI.
		if p < PriorityP0 {
			p = PriorityP0
		} else {
			p = PriorityP3
		}
	}
	return s.store.SetPriority(ctx, slug, p)
}

// Delete elimina una tarjeta por slug.
func (s *Service) Delete(ctx context.Context, slug string) error {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	return s.store.Delete(ctx, slug)
}

// Board devuelve la vista agregada para la página kanban.
func (s *Service) Board(ctx context.Context) (Board, error) {
	board, _, err := s.store.Board(ctx)
	if err != nil {
		return Board{}, err
	}
	return board, nil
}

// UsedSlugs devuelve todos los slugs en uso en una columna. Útil
// para que la UI autocomplete nombres de archivo si hace falta en
// el futuro.
func (s *Service) UsedSlugs(ctx context.Context, dir Status) ([]string, error) {
	cards, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, c := range cards {
		if c.Status == dir {
			out = append(out, c.Slug)
		}
	}
	sort.Strings(out)
	return out, nil
}
