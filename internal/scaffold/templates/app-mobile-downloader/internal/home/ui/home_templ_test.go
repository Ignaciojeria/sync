package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHomePageRendersHelloWorld(t *testing.T) {
	var buf bytes.Buffer
	if err := HomePage().Render(context.Background(), &buf); err != nil {
		t.Fatalf("HomePage().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Hello world") {
		t.Fatalf("expected body to contain 'Hello world', got %q", body)
	}
	if !strings.Contains(body, "Sync 4 Run") {
		t.Fatalf("expected body to contain 'Sync 4 Run', got %q", body)
	}
}
