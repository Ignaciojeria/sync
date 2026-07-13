package server

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"fixtests1/internal/shared"
	"fixtests1/internal/shared/configuration"
)

type fakeRunner struct {
	runErr error
}

func (f fakeRunner) Run() error {
	return f.runErr
}

type fakeShutdownServer struct {
	shutdownErr error
	called      bool
	deadlineSet bool
}

func (f *fakeShutdownServer) Shutdown(ctx context.Context) error {
	f.called = true
	_, f.deadlineSet = ctx.Deadline()
	return f.shutdownErr
}

func TestNew(t *testing.T) {
	s := New(configuration.Conf{PORT: " 8080 "}, nil, nil)
	if s == nil {
		t.Fatal("expected server to be created")
	}
	if s == nil || s.Mux == nil {
		t.Fatal("expected net/http server to be initialized")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := shared.FirstNonEmpty(" ", " value ", "other"); got != "value" {
		t.Fatalf("shared.FirstNonEmpty() = %q", got)
	}
	if got := shared.FirstNonEmpty(" ", "\t"); got != "" {
		t.Fatalf("shared.FirstNonEmpty() = %q, want empty string", got)
	}
}

func TestRunServer(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		runServer(fakeRunner{})
	})

	t.Run("panic on error", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		runServer(fakeRunner{runErr: errors.New("boom")})
	})

	t.Run("does not panic on http.ErrServerClosed", func(t *testing.T) {
		// Reproduce el caso del runner de fuego: tras un Shutdown,
		// http.Server.Serve retorna http.ErrServerClosed. runServer debe tratarlo
		// como salida esperada, no como error.
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("runServer panicked on graceful shutdown: %v", r)
			}
		}()
		runServer(fakeRunner{runErr: http.ErrServerClosed})
	})
}

func TestShutdownHook(t *testing.T) {
	t.Run("returns shutdown error and sets timeout context", func(t *testing.T) {
		server := &fakeShutdownServer{shutdownErr: errors.New("shutdown failed")}
		hook := shutdownHook(server)

		started := time.Now()
		err := hook()
		if err == nil || err.Error() != "shutdown failed" {
			t.Fatalf("unexpected error: %v", err)
		}
		if !server.called {
			t.Fatal("expected Shutdown to be called")
		}
		if !server.deadlineSet {
			t.Fatal("expected deadline to be set on context")
		}
		if time.Since(started) > time.Second {
			t.Fatal("shutdown hook should return quickly in tests")
		}
	})
}

func TestStartServerRegistersShutdownHook(t *testing.T) {
	hooks := &shared.Hooks{}
	server := NewServer(WithAddr(":0"))

	if err := Start(server, hooks); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := hooks.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}
