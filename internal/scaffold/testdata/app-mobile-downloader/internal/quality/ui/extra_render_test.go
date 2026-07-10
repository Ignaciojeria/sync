package ui

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

var _ = bytes.Buffer{}

type failingTemplWriter struct {
	err error
}

func (f failingTemplWriter) Write(p []byte) (int, error) { return 0, f.err }

// Template renders flush their internal buffer to the underlying writer at the
// very end; we exercise that branch directly with a writer that always errors.
func TestDashboardStatsRenderFailingWriter(t *testing.T) {
	state := TestRunState{
		Success:      true,
		Output:       "ok",
		CoverPercent: 88.8,
		Timestamp:    time.Unix(1700000000, 0),
		HasResult:    true,
	}
	err := DashboardStats(state).Render(context.Background(), failingTemplWriter{err: errors.New("flush failure")})
	if err == nil {
		t.Fatal("expected flush error")
	}
}

func TestTestResultRenderFailingWriter(t *testing.T) {
	state := TestRunState{
		Success:      true,
		Output:       "ok",
		CoverPath:    "/report/tests/coverage.html?t=1",
		CoverPercent: 88.8,
		Timestamp:    time.Unix(1700000000, 0),
		HasResult:    true,
	}
	err := TestResult(state.Success, state.Output, state.CoverPath, state.CoverPercent).Render(context.Background(), failingTemplWriter{err: errors.New("flush failure")})
	if err == nil {
		t.Fatal("expected flush error")
	}
}

func TestPageRenderFailingWriter(t *testing.T) {
	state := TestRunState{
		Success:      true,
		Output:       "ok",
		CoverPercent: 88.8,
		HasResult:    true,
	}
	err := Page(state, "").Render(context.Background(), failingTemplWriter{err: errors.New("flush failure")})
	if err == nil {
		t.Fatal("expected flush error")
	}
}

func TestDashboardStatsRenderCancelledContext(t *testing.T) {
	state := TestRunState{HasResult: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := DashboardStats(state).Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestTestResultRenderCancelledContext(t *testing.T) {
	state := TestRunState{HasResult: true, Success: true}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := TestResult(state.Success, state.Output, state.CoverPath, state.CoverPercent).Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestPageRenderCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Page(TestRunState{}, "").Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
