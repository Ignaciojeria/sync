package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	schedulerapp "fixtests1/internal/scheduler/application"
)

func TestFormatTime(t *testing.T) {
	if formatTime(nil) != "Nunca" {
		t.Fatal("expected 'Nunca' for nil time")
	}
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	if got := formatTime(&now); got != "2026-05-15 10:30:00" {
		t.Fatalf("formatTime() = %q", got)
	}
}

func TestJobsPageRendersAllRows(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	configs := []schedulerapp.JobConfig{
		{ID: "a", Name: "alpha", Schedule: "* * * * *", Endpoint: "/a", Enabled: true, Description: "alpha desc", LastRunAt: &now},
		{ID: "b", Name: "beta", Schedule: "0 12 * * *", Endpoint: "/b", Enabled: false, Description: "beta desc"},
	}

	var buf bytes.Buffer
	if err := JobsPage(configs, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobsPage().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "alpha") || !strings.Contains(body, "beta") {
		t.Fatalf("expected both rows in rendered output, got %q", body)
	}
	if !strings.Contains(body, "Activo") || !strings.Contains(body, "Inactivo") {
		t.Fatalf("expected both Enabled/Disabled badge labels, got %q", body)
	}
}

func TestJobsPageEmptyListRendersEmptyTable(t *testing.T) {
	var buf bytes.Buffer
	if err := JobsPage(nil, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobsPage(nil).Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Configuración de Jobs") {
		t.Fatalf("expected title in rendered output, got %q", body)
	}
}

func TestJobFormRendersForm(t *testing.T) {
	var buf bytes.Buffer
	if err := JobForm("").Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobForm().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, `name="name"`) || !strings.Contains(body, `name="schedule"`) {
		t.Fatalf("expected form inputs in rendered output, got %q", body)
	}
}

func TestSchedulerAppPath(t *testing.T) {
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

func TestEmptyFormRendersEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := EmptyForm().Render(context.Background(), &buf); err != nil {
		t.Fatalf("EmptyForm().Render() error = %v", err)
	}
	body := strings.TrimSpace(buf.String())
	if body != "<div></div>" {
		t.Fatalf("expected empty form to render empty div, got %q", body)
	}
}

func TestJobRowDisablesAndEnablesActions(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)

	enabled := schedulerapp.JobConfig{ID: "job-1", Name: "enabled", Schedule: "* * * * *", Endpoint: "/x", Enabled: true, Description: "D", LastRunAt: &now}
	disabled := schedulerapp.JobConfig{ID: "job-1", Name: "enabled", Schedule: "* * * * *", Endpoint: "/x", Enabled: false, Description: "D"}

	var buf bytes.Buffer
	if err := JobRow(enabled, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobRow(enabled).Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "Desactivar") {
		t.Fatalf("expected 'Desactivar' button on enabled job, got %q", body)
	}
	if !strings.Contains(body, "Activo") {
		t.Fatal("expected 'Activo' badge on enabled job")
	}

	buf.Reset()
	if err := JobRow(disabled, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("JobRow(disabled).Render() error = %v", err)
	}
	body = buf.String()
	if !strings.Contains(body, "Activar") {
		t.Fatalf("expected 'Activar' button on disabled job, got %q", body)
	}
	if !strings.Contains(body, "Inactivo") {
		t.Fatal("expected 'Inactivo' badge on disabled job")
	}
	if !strings.Contains(body, "Nunca") {
		t.Fatal("expected 'Nunca' for nil LastRunAt")
	}
}
