package ui

import (
	"testing"
	"time"

	topologyapp "gitinittest5/internal/topology/application"
)

func TestStatusBadgeClass(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{topologyapp.StatusRunning, "badge badge-success badge-outline"},
		{topologyapp.StatusSyncing, "badge badge-info badge-outline"},
		{topologyapp.StatusDegraded, "badge badge-warning badge-outline"},
		{topologyapp.StatusOffline, "badge badge-error badge-outline"},
		{"unknown", "badge badge-ghost"},
		{"", "badge badge-ghost"},
	}
	for _, c := range cases {
		if got := statusBadgeClass(c.in); got != c.want {
			t.Errorf("statusBadgeClass(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatStatus(t *testing.T) {
	if got := formatStatus(""); got != "unknown" {
		t.Errorf("empty = %q", got)
	}
	if got := formatStatus("  "); got != "unknown" {
		t.Errorf("whitespace = %q", got)
	}
	if got := formatStatus("running"); got != "Running" {
		t.Errorf("running = %q", got)
	}
	if got := formatStatus("offline"); got != "Offline" {
		t.Errorf("offline = %q", got)
	}
}

func TestGeneratedLabel(t *testing.T) {
	if got := generatedLabel(time.Time{}); got != "ahora" {
		t.Errorf("zero = %q", got)
	}
	now := time.Date(2026, 6, 26, 18, 0, 0, 0, time.UTC)
	got := generatedLabel(now)
	if got == "ahora" {
		t.Errorf("non-zero should not be ahora: %q", got)
	}
}

func TestSplitSessionClientName(t *testing.T) {
	cases := []struct {
		in                string
		primary, secondary string
	}{
		{"user@host · BOX-1", "user@host", "BOX-1"},
		{"only", "only", ""},
		{"", "", ""},
		{"  user@host · BOX-1  ", "user@host", "BOX-1"}, // ponytail: la función trimea el input completo antes de split.
		{"  ·  only-second", "·  only-second", ""},      // sin " · " con espacios → primary = todo, secondary = ""
	}
	for _, c := range cases {
		p, s := splitSessionClientName(c.in)
		if p != c.primary || s != c.secondary {
			t.Errorf("splitSessionClientName(%q) = (%q, %q), want (%q, %q)", c.in, p, s, c.primary, c.secondary)
		}
	}
}

func TestSessionLabels(t *testing.T) {
	if got := sessionPrimaryLabel("user@host · BOX"); got != "user@host" {
		t.Errorf("primary = %q", got)
	}
	if got := sessionSecondaryLabel("user@host · BOX"); got != "BOX" {
		t.Errorf("secondary = %q", got)
	}
	if got := sessionPrimaryLabel("solo"); got != "solo" {
		t.Errorf("primary sin separador = %q", got)
	}
}
