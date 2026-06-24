package ui

import (
	"context"
	"errors"
	"testing"
)

type failingTemplEditorWriter struct{ err error }

func (f failingTemplEditorWriter) Write(p []byte) (int, error) { return 0, f.err }

func TestEditorViewRenderErrorBranches(t *testing.T) {
	w := failingTemplEditorWriter{err: errors.New("flush failure")}
	if err := EditorView().Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from EditorView")
	}
}

func TestEditorViewCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := EditorView().Render(ctx, &failingTemplEditorWriter{}); err == nil {
		t.Fatal("expected error from cancelled context in EditorView")
	}
}
