package ui

import (
	"context"
	"errors"
	"testing"
	"time"

	schedulerapp "gitinittest5/internal/scheduler/application"
)

type failingTemplWriter struct{ err error }

func (f failingTemplWriter) Write(p []byte) (int, error) { return 0, f.err }

func TestSchedulerTemplRenderErrorBranches(t *testing.T) {
	w := failingTemplWriter{err: errors.New("flush failure")}
	now := time.Now()

	if err := JobsPage([]schedulerapp.JobConfig{}, "").Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from JobsPage")
	}
	w.err = errors.New("flush2")
	if err := JobForm("").Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from JobForm")
	}
	w.err = errors.New("flush3")
	if err := EmptyForm().Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from EmptyForm")
	}
	w.err = errors.New("flush4")
	if err := JobRow(schedulerapp.JobConfig{ID: "1", Name: "x", LastRunAt: &now}, "").Render(context.Background(), w); err == nil {
		t.Fatal("expected flush error from JobRow")
	}
}

func TestSchedulerTemplCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := JobsPage(nil, "").Render(ctx, nil); err == nil {
		t.Fatal("expected error from cancelled context in JobsPage")
	}
	if err := JobForm("").Render(ctx, nil); err == nil {
		t.Fatal("expected error from cancelled context in JobForm")
	}
	if err := EmptyForm().Render(ctx, nil); err == nil {
		t.Fatal("expected error from cancelled context in EmptyForm")
	}
	if err := JobRow(schedulerapp.JobConfig{ID: "1"}, "").Render(ctx, nil); err == nil {
		t.Fatal("expected error from cancelled context in JobRow")
	}
}
