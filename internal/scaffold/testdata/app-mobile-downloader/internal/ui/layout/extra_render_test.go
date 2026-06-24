package layout

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/a-h/templ"
)

type failingTemplWriter struct{ err error }

func (f failingTemplWriter) Write(p []byte) (int, error) { return 0, f.err }

func TestLayoutRenderErrorBranches(t *testing.T) {
	w := failingTemplWriter{err: errors.New("flush failure")}

	if err := Layout("T", "ocean", "/design/theme/ocean").Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from Layout")
	}
	w.err = errors.New("flush2")
	if err := LayoutWithNav("T", themedNav("/", false)).Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from LayoutWithNav")
	}
	w.err = errors.New("flush3")
	if err := SideNav(themedNav("/", false)).Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from SideNav")
	}
}

func TestLayoutRenderCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Layout("T", "ocean", "/design/theme/ocean").Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context in Layout")
	}
	if err := LayoutWithNav("T", themedNav("/", false)).Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context in LayoutWithNav")
	}
	if err := SideNav(themedNav("/", false)).Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context in SideNav")
	}
}

func TestRenderPageWithCancelledContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := minimalCtx{req: req, w: rec}

	page, err := RenderPage(c, "T", templ.Raw("body"))
	if err != nil {
		t.Fatalf("RenderPage() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := page.Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context during page render")
	}
}

func TestRenderPageFailingWriter(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := minimalCtx{req: req, w: rec}

	page, err := RenderPage(c, "T", templ.Raw("body"))
	if err != nil {
		t.Fatalf("RenderPage() error = %v", err)
	}
	w := failingTemplWriter{err: errors.New("flush failure")}
	if err := page.Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from RenderPage")
	}
}

var _ time.Time
