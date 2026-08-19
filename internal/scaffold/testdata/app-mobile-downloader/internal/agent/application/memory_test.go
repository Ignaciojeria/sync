package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNoopProvider_EnsurePeers(t *testing.T) {
	t.Parallel()
	var p MemoryProvider = noopProvider{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := p.EnsurePeers(ctx, MemoryKey{SessionID: "x"}); err != nil {
		t.Fatalf("noop EnsurePeers should never error, got %v", err)
	}
}

func TestNoopProvider_Recall(t *testing.T) {
	t.Parallel()
	var p MemoryProvider = noopProvider{}
	got, err := p.Recall(context.Background(), MemoryKey{SessionID: "x"}, "cualquier cosa")
	if err != nil {
		t.Fatalf("noop Recall should never error, got %v", err)
	}
	if got.Text != "" {
		t.Errorf("noop Recall should return empty Text, got %q", got.Text)
	}
	if got.TokensUsed != 0 {
		t.Errorf("noop Recall should return 0 TokensUsed, got %d", got.TokensUsed)
	}
}

func TestNoopProvider_Remember_EmptySlice(t *testing.T) {
	t.Parallel()
	var p MemoryProvider = noopProvider{}
	if err := p.Remember(context.Background(), MemoryKey{SessionID: "x"}, nil); err != nil {
		t.Fatalf("noop Remember(nil) should never error, got %v", err)
	}
	if err := p.Remember(context.Background(), MemoryKey{SessionID: "x"}, []MemoryMessage{}); err != nil {
		t.Fatalf("noop Remember([]) should never error, got %v", err)
	}
}

func TestNoopProvider_Remember_NonEmpty(t *testing.T) {
	t.Parallel()
	var p MemoryProvider = noopProvider{}
	msgs := []MemoryMessage{
		{Role: "user", Text: "hola", CreatedAt: time.Now()},
		{Role: "assistant", Text: "chau", CreatedAt: time.Now()},
	}
	if err := p.Remember(context.Background(), MemoryKey{SessionID: "x"}, msgs); err != nil {
		t.Fatalf("noop Remember(msgs) should never error, got %v", err)
	}
}

// TestNoopProvider_HonorsContextCancellation es defensivo: aunque el
// noop no hace I/O y no debería bloquearse, si por error futuro alguien
// agrega un sleep, queremos que ctx.Done lo corte. El noop actual no
// chequea ctx — esto es más un guard para futuras implementaciones
// que un test del noop.
func TestNoopProvider_HonorsContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ya cancelado
	// El noop ignora ctx; esto verifica que sigue siendo no-op.
	if err := (noopProvider{}).EnsurePeers(ctx, MemoryKey{}); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("noop with canceled ctx should be a no-op, got %v", err)
	}
}
