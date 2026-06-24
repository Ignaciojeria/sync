package ui

import (
	"context"
	"errors"
	"testing"

	"github.com/a-h/templ"
)

type failingTemplWriter struct{ err error }

func (f failingTemplWriter) Write(p []byte) (int, error) { return 0, f.err }

// TestLoginPageFailingWriter drives the templ-generated WriteString error
// branch that ultimately surfaces as a non-nil render error.
func TestLoginPageFailingWriter(t *testing.T) {
	w := failingTemplWriter{err: errors.New("flush failure")}
	if err := LoginPage().Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from LoginPage")
	}
}

// TestLoginPageChildren exercises the children branch of the generated template.
func TestLoginPageChildren(t *testing.T) {
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := LoginPage().Render(ctx, &failingTemplWriter{err: errors.New("flush")}); err == nil {
		t.Fatal("expected flush error with children set")
	}
}
