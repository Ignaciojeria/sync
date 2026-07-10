package application

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManagerRegisterPreview_SetsPreviewFields(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	ctx := t.Context()

	session, err := manager.Create(ctx, CreateSessionInput{Title: "preview", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/healthz"; got != want {
			t.Fatalf("health path = %q, want %q", got, want)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	port := serverPort(t, srv.Listener.Addr())
	updated, err := manager.RegisterPreview(ctx, session.ID, RegisterPreviewInput{Port: port, HealthPath: "/healthz"})
	if err != nil {
		t.Fatalf("RegisterPreview: %v", err)
	}
	if got, want := updated.PreviewPort, port; got != want {
		t.Fatalf("PreviewPort = %d, want %d", got, want)
	}
	if got, want := updated.PreviewStatus, PreviewStatusLive; got != want {
		t.Fatalf("PreviewStatus = %q, want %q", got, want)
	}
	if got, want := updated.PreviewURL, previewPublicURL(session.ID); got != want {
		t.Fatalf("PreviewURL = %q, want %q", got, want)
	}
}

func TestManagerRegisterPreview_RejectsUnavailableUpstream(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	ctx := t.Context()

	session, err := manager.Create(ctx, CreateSessionInput{Title: "preview", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = manager.RegisterPreview(ctx, session.ID, RegisterPreviewInput{Port: 1, HealthPath: "/"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrPreviewUnavailable) {
		t.Fatalf("err = %v, want ErrPreviewUnavailable", err)
	}
}

func TestHealthcheckPreview_Accepts401Rejects500(t *testing.T) {
	unauthorized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer unauthorized.Close()
	if status, err := HealthcheckPreview(serverPort(t, unauthorized.Listener.Addr()), "/"); err != nil || status != http.StatusUnauthorized {
		t.Fatalf("401 healthcheck = (%d, %v), want (%d, nil)", status, err, http.StatusUnauthorized)
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()
	if status, err := HealthcheckPreview(serverPort(t, broken.Listener.Addr()), "/"); err == nil || status != http.StatusInternalServerError {
		t.Fatalf("500 healthcheck = (%d, %v), want error", status, err)
	}
}

func TestManagerClearPreview_RemovesPreviewFields(t *testing.T) {
	store := newStubStore()
	manager := NewManager(store, &factoryRunner{})
	ctx := t.Context()

	session, err := manager.Create(ctx, CreateSessionInput{Title: "preview", CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	stored, err := store.Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	stored.PreviewPort = 43123
	stored.PreviewStatus = PreviewStatusLive
	stored.PreviewHealth = "/"
	stored.PreviewURL = previewPublicURL(session.ID)
	if err := store.Update(ctx, stored); err != nil {
		t.Fatalf("store.Update: %v", err)
	}

	cleared, err := manager.ClearPreview(ctx, session.ID)
	if err != nil {
		t.Fatalf("ClearPreview: %v", err)
	}
	if cleared.PreviewPort != 0 || cleared.PreviewURL != "" || cleared.PreviewHealth != "" || cleared.PreviewStatus != PreviewStatusNone {
		t.Fatalf("preview not cleared: %#v", cleared)
	}
}

func serverPort(t *testing.T, addr net.Addr) int {
	t.Helper()
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type = %T", addr)
	}
	return tcp.Port
}
