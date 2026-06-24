package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEditorViewRendersIframe(t *testing.T) {
	var buf bytes.Buffer
	if err := EditorView().Render(context.Background(), &buf); err != nil {
		t.Fatalf("EditorView().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, `src="/editor"`) {
		t.Fatalf("expected iframe src to /editor, got %q", body)
	}
	if !strings.Contains(body, "sandbox=") {
		t.Fatalf("expected iframe to declare sandbox, got %q", body)
	}
}
