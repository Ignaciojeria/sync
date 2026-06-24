package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHomePageRendersContent(t *testing.T) {
	var buf bytes.Buffer
	if err := HomePage().Render(context.Background(), &buf); err != nil {
		t.Fatalf("HomePage().Render() error = %v", err)
	}
	body := buf.String()
	checks := []string{
		"Sync 4 Run",
		"Tu workspace persistente para operar agentes.",
		"Abrir consola",
		"Ver design system",
		"Consola",
		"Calidad",
		"Jobs",
		"Tema",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %q", want, body)
		}
	}
}
