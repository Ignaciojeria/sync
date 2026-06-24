package ui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	designapp "app-mobile-downloader/internal/design/application"

	"github.com/a-h/templ"
)

func TestPanelRendersLink(t *testing.T) {
	var buf bytes.Buffer
	if err := Panel([]designapp.ResolvedTheme{{ID: "ocean", Name: "Ocean"}}, designapp.ResolvedTheme{ID: "ocean", Name: "Ocean"}).Render(context.Background(), &buf); err != nil {
		t.Fatalf("Panel().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Theme") {
		t.Fatalf("expected Theme title, got %q", body)
	}
	if !strings.Contains(body, "href=\"/design\"") {
		t.Fatalf("expected design link, got %q", body)
	}
	if !strings.Contains(body, "Design system") {
		t.Fatalf("expected design label, got %q", body)
	}
}

func TestPanelErrorAndContextBranches(t *testing.T) {
	w := failingWriter{err: errors.New("flush")}
	if err := Panel(nil, designapp.ResolvedTheme{}).Render(context.Background(), w); err == nil {
		t.Fatal("expected Panel render error")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Panel(nil, designapp.ResolvedTheme{}).Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected Panel cancelled context error")
	}
}

func TestPanelRendersWithChildrenInContext(t *testing.T) {
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	var buf bytes.Buffer
	if err := Panel(nil, designapp.ResolvedTheme{}).Render(ctx, &buf); err != nil {
		t.Fatalf("Panel().Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Design system") {
		t.Fatalf("expected rendered panel, got %q", buf.String())
	}
}
