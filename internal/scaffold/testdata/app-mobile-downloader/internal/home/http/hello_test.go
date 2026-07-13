package home

import (
	"context"
	"fixtests1/internal/shared/server"
	topologyapp "fixtests1/internal/topology/application"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type snapshotReaderStub struct {
	snapshot topologyapp.Snapshot
}

func (s snapshotReaderStub) GetSnapshot(context.Context) (topologyapp.Snapshot, error) {
	return s.snapshot, nil
}

func TestHomeHandler(t *testing.T) {
	fs := server.NewServer()
	s := fs
	homeHandler(s, snapshotReaderStub{snapshot: topologyapp.Snapshot{
		Workspace: topologyapp.Workspace{Name: "workspace-gateway", Status: topologyapp.StatusRunning, Summary: "Runtime persistente del workspace"},
		Services:  []topologyapp.ServiceNode{{Name: "PostgreSQL", Kind: "database", Status: topologyapp.StatusRunning, Summary: "Database connection healthy"}},
	}})
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q", got)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	bodyText := string(body)
	checks := []string{
		"workspace-gateway",
		"Topology",
		"PostgreSQL",
		"Abrir consola",
	}
	for _, want := range checks {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("expected body to contain %q, got %q", want, bodyText)
		}
	}
}

func TestHomeRegister(t *testing.T) {
	fs := server.NewServer()
	s := fs
	Register(s, snapshotReaderStub{snapshot: topologyapp.Snapshot{
		Workspace: topologyapp.Workspace{Name: "workspace-gateway", Status: topologyapp.StatusRunning, Summary: "Runtime persistente del workspace"},
		Services:  []topologyapp.ServiceNode{{Name: "PostgreSQL", Kind: "database", Status: topologyapp.StatusRunning, Summary: "Database connection healthy"}},
	}})
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}
