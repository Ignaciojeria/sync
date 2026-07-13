package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPageRendersInitialAndResult(t *testing.T) {
	t.Run("no result yet shows initial state", func(t *testing.T) {
		var buf bytes.Buffer
		if err := Page(TestRunState{}, "").Render(context.Background(), &buf); err != nil {
			t.Fatalf("Page().Render() error = %v", err)
		}
		body := buf.String()
		if !strings.Contains(body, "Reporte de Tests") {
			t.Fatalf("expected title text, got %q", body)
		}
		if !strings.Contains(body, "Panel de control de calidad y cobertura") {
			t.Fatalf("expected subtitle, got %q", body)
		}
		if !strings.Contains(body, "Presiona") {
			t.Fatalf("expected placeholder text, got %q", body)
		}
	})

	t.Run("with result shows test result block", func(t *testing.T) {
		now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
		var buf bytes.Buffer
		if err := Page(TestRunState{
			Success:      true,
			Output:       "ok",
			CoverPath:    "/report/tests/coverage.html?t=1",
			CoverPercent: 88.5,
			Timestamp:    now,
			HasResult:    true,
		}, "").Render(context.Background(), &buf); err != nil {
			t.Fatalf("Page().Render() error = %v", err)
		}
		body := buf.String()
		if !strings.Contains(body, "88.5%") {
			t.Fatalf("expected cover percent in body, got %q", body)
		}
		if !strings.Contains(body, "Resume") && !strings.Contains(body, "Resumen") {
			t.Fatalf("expected resumen section, got %q", body)
		}
	})

	t.Run("test button attributes injected via templ", func(t *testing.T) {
		attrs := map[string]any(testButtonAttrs(""))
		if attrs["hx-post"] != "/report/tests/run" {
			t.Fatalf("hx-post = %v", attrs["hx-post"])
		}
		if got := attrs["hx-target"]; got != "#test-result" {
			t.Fatalf("hx-target = %v", got)
		}
		if got := attrs["hx-swap"]; got != "innerHTML" {
			t.Fatalf("hx-swap = %v", got)
		}
	})
}

func TestQualityAppPath(t *testing.T) {
	cases := []struct {
		prefix, path, want string
	}{
		{"", "/foo", "/foo"},
		{"", "foo", "/foo"},
		{"/agent", "/foo", "/agent/foo"},
		{"/agent", "/", "/agent/"},
		{"  ", "/x", "/x"},
	}
	for _, c := range cases {
		if got := appPath(c.prefix, c.path); got != c.want {
			t.Errorf("appPath(%q, %q) = %q, want %q", c.prefix, c.path, got, c.want)
		}
	}
}
