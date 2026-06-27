package topology

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"app-mobile-downloader/internal/shared/server"
	topologyapp "app-mobile-downloader/internal/topology/application"
	memory "app-mobile-downloader/internal/topology/infrastructure/memory"

	"github.com/go-fuego/fuego"
)

func TestUpsertSyncSessionHandler(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	store := memory.NewSyncSessionsStore(time.Minute)
	service := topologyapp.NewServiceWithDeps(topologyapp.ServiceDeps{SyncSessionsStore: store, SyncSessionsSource: store})
	Register(s, service)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	body := []byte(`{"session_id":"s1","project_name":"workspace-gateway","client_name":"ignacio-laptop","source":"mutagen","status":"running"}`)
	res, err := http.Post(ts.URL+"/api/topology/sync-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}

	snapshot, err := service.GetSnapshot(t.Context())
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if len(snapshot.SyncSessions) != 1 {
		t.Fatalf("len(sync sessions) = %d", len(snapshot.SyncSessions))
	}
}

func TestUpsertSyncSessionHandlerRejectsBadRequest(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	store := memory.NewSyncSessionsStore(time.Minute)
	service := topologyapp.NewServiceWithDeps(topologyapp.ServiceDeps{SyncSessionsStore: store, SyncSessionsSource: store})
	Register(s, service)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	body := []byte(`{"project_name":"workspace-gateway"}`)
	res, err := http.Post(ts.URL+"/api/topology/sync-sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", res.StatusCode)
	}
}
