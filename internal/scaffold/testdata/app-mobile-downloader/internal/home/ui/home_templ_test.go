package ui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	topologyapp "fixtests1/internal/topology/application"
)

func TestHomePageRendersContent(t *testing.T) {
	snapshot := topologyapp.Snapshot{
		Workspace:    topologyapp.Workspace{Name: "workspace-gateway", Status: topologyapp.StatusRunning, Summary: "Runtime persistente del workspace"},
		Services:     []topologyapp.ServiceNode{{Name: "PostgreSQL", Kind: "database", Status: topologyapp.StatusRunning, Summary: "Database connection healthy"}},
		SyncSessions: []topologyapp.SyncSession{{SessionID: "abc", ProjectName: "workspace-gateway", ClientName: "ignaciovl.j@gmail.com", Status: topologyapp.StatusSyncing, Source: "mutagen"}},
	}
	var buf bytes.Buffer
	if err := HomePage(snapshot, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("HomePage().Render() error = %v", err)
	}
	body := buf.String()
	checks := []string{
		"workspace-gateway",
		"Workspace runtime",
		"Topology",
		"PostgreSQL",
		"Sync session",
		"Infrastructure",
		"ignaciovl.j@gmail.com",
		"Abrir consola",
		"Calidad",
		"Jobs",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected body to contain %q, got %q", want, body)
		}
	}
}

func TestHomePageAppPath(t *testing.T) {
	cases := []struct {
		prefix, path, want string
	}{
		{"", "/foo", "/foo"},
		{"", "foo", "/foo"},
		{"/agent", "/foo", "/agent/foo"},
		{"/agent", "/", "/agent/"},
		{"/agent/", "/foo", "/agent/foo"},
		{"  ", "/x", "/x"},
	}
	for _, c := range cases {
		if got := appPath(c.prefix, c.path); got != c.want {
			t.Errorf("appPath(%q, %q) = %q, want %q", c.prefix, c.path, got, c.want)
		}
	}
}
