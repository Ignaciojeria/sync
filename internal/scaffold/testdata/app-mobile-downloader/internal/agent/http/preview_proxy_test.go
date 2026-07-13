package agent

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentapp "fixtests1/internal/agent/application"
)

type previewServiceStub struct {
	session agentapp.Session
	err     error
}

func (s previewServiceStub) List(context.Context) ([]agentapp.Session, error) { return nil, nil }
func (s previewServiceStub) Create(context.Context, agentapp.CreateSessionInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s previewServiceStub) Get(context.Context, string) (agentapp.Session, error) {
	return s.session, s.err
}
func (s previewServiceStub) Ensure(context.Context, string) error         { return nil }
func (s previewServiceStub) Prompt(context.Context, string, string) error { return nil }
func (s previewServiceStub) PromptRequest(context.Context, string, agentapp.PromptInput) error {
	return nil
}
func (s previewServiceStub) Steer(context.Context, string, string) error { return nil }
func (s previewServiceStub) Abort(context.Context, string) error         { return nil }
func (s previewServiceStub) Subscribe(context.Context, string) (<-chan agentapp.Event, func(), error) {
	return nil, func() {}, nil
}
func (s previewServiceStub) RegisterPreview(context.Context, string, agentapp.RegisterPreviewInput) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s previewServiceStub) ClearPreview(context.Context, string) (agentapp.Session, error) {
	return agentapp.Session{}, nil
}
func (s previewServiceStub) ApplyPreview(context.Context, string) (agentapp.ApplyResult, error) {
	return agentapp.ApplyResult{}, nil
}
func (s previewServiceStub) MergePreview(context.Context, string) (agentapp.MergeResult, error) {
	return agentapp.MergeResult{}, nil
}
func (s previewServiceStub) Delete(context.Context, string) error { return nil }
func (s previewServiceStub) Close() error                         { return nil }

func TestPreviewProxy_ProxiesPathAndQuery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/assets/app.js"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.RawQuery, "v=42"; got != want {
			t.Fatalf("query = %q, want %q", got, want)
		}
		if got := r.Header.Get(previewProxyHeader); got != "1" {
			t.Fatalf("missing %s header", previewProxyHeader)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	port := upstreamPort(t, upstream.Listener.Addr())
	handler := http.HandlerFunc(previewProxy(previewServiceStub{session: agentapp.Session{
		ID:            "s1",
		PreviewPort:   port,
		PreviewStatus: agentapp.PreviewStatusLive,
	}}))

	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/preview/assets/app.js?v=42", nil)
	req.SetPathValue("id", "s1")
	req.SetPathValue("rest", "assets/app.js")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d, body=%s", got, want, rec.Body.String())
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body = %q, want ok", got)
	}
}

func TestPreviewProxy_RejectsLoopHeader(t *testing.T) {
	handler := http.HandlerFunc(previewProxy(previewServiceStub{}))
	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/preview", nil)
	req.Header.Set(previewProxyHeader, "1")
	req.SetPathValue("id", "s1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusLoopDetected; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func TestPreviewProxy_RewritesRelativeLocationHeaderToMountedPrefix(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/auth/login", http.StatusFound)
	}))
	defer upstream.Close()

	port := upstreamPort(t, upstream.Listener.Addr())
	handler := http.HandlerFunc(previewProxy(previewServiceStub{session: agentapp.Session{
		ID:            "s1",
		PreviewPort:   port,
		PreviewStatus: agentapp.PreviewStatusLive,
	}}))

	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/preview/private", nil)
	req.SetPathValue("id", "s1")
	req.SetPathValue("rest", "private")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Location"), "/agent/sessions/s1/preview/auth/login"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func TestPreviewProxy_DoesNotDoublePrefixMountedLocationHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/agent/sessions/s1/preview/agent?session=s1")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	port := upstreamPort(t, upstream.Listener.Addr())
	handler := http.HandlerFunc(previewProxy(previewServiceStub{session: agentapp.Session{
		ID:            "s1",
		PreviewPort:   port,
		PreviewStatus: agentapp.PreviewStatusLive,
	}}))

	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/preview/agent", nil)
	req.SetPathValue("id", "s1")
	req.SetPathValue("rest", "agent")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := rec.Header().Get("Location"), "/agent/sessions/s1/preview/agent?session=s1"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}

func TestPreviewProxy_ReturnsNotFoundWhenPreviewMissing(t *testing.T) {
	handler := http.HandlerFunc(previewProxy(previewServiceStub{session: agentapp.Session{ID: "s1"}}))
	req := httptest.NewRequest(http.MethodGet, "/agent/sessions/s1/preview", nil)
	req.SetPathValue("id", "s1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if got, want := rec.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func upstreamPort(t *testing.T, addr net.Addr) int {
	t.Helper()
	tcp, ok := addr.(*net.TCPAddr)
	if !ok {
		t.Fatalf("addr type = %T", addr)
	}
	return tcp.Port
}

var _ agentapp.AgentService = previewServiceStub{}
var _ = time.Second
