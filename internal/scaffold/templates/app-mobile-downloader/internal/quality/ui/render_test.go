package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type failAfterWriter struct {
	written   int
	failAfter int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.written >= w.failAfter {
		return 0, w.err
	}
	remaining := w.failAfter - w.written
	if len(p) > remaining {
		w.written += remaining
		return remaining, w.err
	}
	w.written += len(p)
	return len(p), nil
}

func TestRenderResultAndDashboard(t *testing.T) {
	state := TestRunState{
		Success:      true,
		Output:       "ok output",
		CoverPath:    "/report/tests/coverage.html?t=1",
		CoverPercent: 88.8,
		Timestamp:    time.Unix(1700000000, 0),
		HasResult:    true,
	}

	t.Run("success", func(t *testing.T) {
		var out bytes.Buffer
		err := RenderResultAndDashboard(&out, context.Background(), state)
		if err != nil {
			t.Fatalf("RenderResultAndDashboard() error = %v", err)
		}
		rendered := out.String()
		if !strings.Contains(rendered, "Resumen") {
			t.Fatalf("expected rendered output to include summary section, got %q", rendered)
		}
	})

	t.Run("dashboard render error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := RenderResultAndDashboard(io.Discard, ctx, state)
		if err == nil {
			t.Fatal("expected error from dashboard render")
		}
	})

	t.Run("test result render error", func(t *testing.T) {
		var dashboardOut bytes.Buffer
		if err := DashboardStats(state).Render(context.Background(), &dashboardOut); err != nil {
			t.Fatalf("DashboardStats render setup failed: %v", err)
		}

		w := &failAfterWriter{
			failAfter: dashboardOut.Len(),
			err:       errors.New("forced write error"),
		}

		err := RenderResultAndDashboard(w, context.Background(), state)
		if err == nil {
			t.Fatal("expected error from test result render")
		}
		if !strings.Contains(err.Error(), "forced write error") {
			t.Fatalf("expected forced write error, got %v", err)
		}
	})
}
