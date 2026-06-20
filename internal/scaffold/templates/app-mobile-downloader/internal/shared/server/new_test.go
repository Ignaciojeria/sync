package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/configuration"

	"github.com/go-fuego/fuego"
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

type fakeShutdowner struct {
	hooks []func() error
}

func (f *fakeShutdowner) RegisterShutdown(hook func() error) {
	f.hooks = append(f.hooks, hook)
}

func TestNew(t *testing.T) {
	s := New(configuration.Conf{PORT: " 8080 "}, nil, nil)
	if s == nil {
		t.Fatal("expected server to be created")
	}
	if s.Server == nil {
		t.Fatal("expected embedded fuego server to be initialized")
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
	shutdowner := &fakeShutdowner{}
	server := &Server{Server: fuego.NewServer(fuego.WithAddr(":0"))}

	if err := startServer(server, shutdowner); err != nil {
		t.Fatalf("startServer() error = %v", err)
	}
	if len(shutdowner.hooks) != 1 {
		t.Fatalf("expected 1 shutdown hook, got %d", len(shutdowner.hooks))
	}
	if err := shutdowner.hooks[0](); err != nil {
		t.Fatalf("shutdown hook returned error: %v", err)
	}
}

