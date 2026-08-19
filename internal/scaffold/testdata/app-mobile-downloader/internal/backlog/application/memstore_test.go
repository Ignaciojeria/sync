package application

import (
	"context"
	"sync"
)

// memStore es una implementación en memoria de Store, solo para
// tests de Service. NO se usa en producción (el FS store vive en
// infrastructure/fs).
type memStore struct {
	mu    sync.Mutex
	cards map[string]Card
}

func newMemStore() *memStore {
	return &memStore{cards: map[string]Card{}}
}

func (m *memStore) List(_ context.Context) ([]Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Card, 0, len(m.cards))
	for _, c := range m.cards {
		out = append(out, c)
	}
	return out, nil
}

func (m *memStore) Get(_ context.Context, slug string) (Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cards[slug]
	if !ok {
		return Card{}, ErrNotFound
	}
	return c, nil
}

func (m *memStore) Create(_ context.Context, dir Status, card Card) (Card, error) {
	if !dir.Valid() {
		return Card{}, ErrInvalidInput
	}
	card.Type = DefaultType
	card.Status = dir
	m.mu.Lock()
	defer m.mu.Unlock()
	card.Slug = Slugify(card.Title, m.usedSlugs())
	m.cards[card.Slug] = card
	return card, nil
}

func (m *memStore) Update(_ context.Context, slug string, card Card) (Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cards[slug]; !ok {
		return Card{}, ErrNotFound
	}
	// Slug puede cambiar si title cambió.
	delete(m.cards, slug)
	card.Slug = Slugify(card.Title, m.usedSlugs())
	m.cards[card.Slug] = card
	return card, nil
}

func (m *memStore) Move(_ context.Context, slug string, to Status) (Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cards[slug]
	if !ok {
		return Card{}, ErrNotFound
	}
	c.Status = to
	m.cards[slug] = c
	return c, nil
}

func (m *memStore) SetPriority(_ context.Context, slug string, p Priority) (Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cards[slug]
	if !ok {
		return Card{}, ErrNotFound
	}
	c.Priority = p
	m.cards[slug] = c
	return c, nil
}

func (m *memStore) Delete(_ context.Context, slug string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.cards[slug]; !ok {
		return ErrNotFound
	}
	delete(m.cards, slug)
	return nil
}

func (m *memStore) Board(ctx context.Context) (Board, []InvalidCard, error) {
	cards, err := m.List(ctx)
	if err != nil {
		return Board{}, nil, err
	}
	board, inv := ToBoard(cards)
	return board, inv, nil
}

func (m *memStore) usedSlugs() map[string]bool {
	out := map[string]bool{}
	for _, c := range m.cards {
		out[c.Slug] = true
	}
	return out
}
