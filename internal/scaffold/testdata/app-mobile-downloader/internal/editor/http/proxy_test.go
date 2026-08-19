package editor

import (
	"crypto/tls"
	"errors"
	"gitinittest5/internal/shared/server"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEditorUpstreamURL(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		t.Setenv("EDITOR_UPSTREAM_URL", "")
		if got := editorUpstreamURL(); got != "http://127.0.0.1:9090" {
			t.Fatalf("editorUpstreamURL() = %q", got)
		}
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv("EDITOR_UPSTREAM_URL", "  http://localhost:9999  ")
		if got := editorUpstreamURL(); got != "http://localhost:9999" {
			t.Fatalf("editorUpstreamURL() = %q", got)
		}
	})
}

func TestRewriteEditorRequest(t *testing.T) {
	t.Run("trims editor prefix", func(t *testing.T) {
		target, _ := url.Parse("http://localhost:9090")
		originalDirector := func(r *http.Request) {
			r.URL.Scheme = target.Scheme
			r.URL.Host = target.Host
		}
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor/assets/app.js", nil)
		req.Header.Set("Host", "example.com")

		rewriteEditorRequest(target, originalDirector, req)

		if req.URL.Path != "/assets/app.js" {
			t.Fatalf("path = %q, want /assets/app.js", req.URL.Path)
		}
		if req.Host != "localhost:9090" {
			t.Fatalf("host = %q, want localhost:9090", req.Host)
		}
		if req.Header.Get("X-Forwarded-Prefix") != "/editor" {
			t.Fatalf("X-Forwarded-Prefix = %q", req.Header.Get("X-Forwarded-Prefix"))
		}
		if req.Header.Get("X-Forwarded-Host") != "example.com" {
			t.Fatalf("X-Forwarded-Host = %q", req.Header.Get("X-Forwarded-Host"))
		}
		if req.Header.Get("X-Forwarded-Proto") != "http" {
			t.Fatalf("X-Forwarded-Proto = %q", req.Header.Get("X-Forwarded-Proto"))
		}
	})

	t.Run("editor root rewrites to slash", func(t *testing.T) {
		target, _ := url.Parse("http://localhost:9090")
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor", nil)

		rewriteEditorRequest(target, func(r *http.Request) {}, req)

		if req.URL.Path != "/" {
			t.Fatalf("path = %q, want /", req.URL.Path)
		}
		if req.URL.RawPath != "/" {
			t.Fatalf("raw path = %q, want /", req.URL.RawPath)
		}
	})

	t.Run("preserves existing prefix header", func(t *testing.T) {
		target, _ := url.Parse("http://localhost:9090")
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor/", nil)
		req.Header.Set("X-Forwarded-Prefix", "/custom")

		rewriteEditorRequest(target, func(r *http.Request) {}, req)

		if req.Header.Get("X-Forwarded-Prefix") != "/custom" {
			t.Fatalf("X-Forwarded-Prefix = %q", req.Header.Get("X-Forwarded-Prefix"))
		}
	})
}

func TestForwardedProto(t *testing.T) {
	t.Run("prefers forwarded proto header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		if got := forwardedProto(req); got != "https" {
			t.Fatalf("forwardedProto() = %q", got)
		}
	})

	t.Run("https when tls present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor", nil)
		req.TLS = &tls.ConnectionState{}
		if got := forwardedProto(req); got != "https" {
			t.Fatalf("forwardedProto() = %q", got)
		}
	})

	t.Run("http by default", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com/editor", nil)
		if got := forwardedProto(req); got != "http" {
			t.Fatalf("forwardedProto() = %q", got)
		}
	})
}

func TestHandleEditorProxyError(t *testing.T) {
	rr := httptest.NewRecorder()
	handleEditorProxyError(rr, errors.New("upstream down"))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "editor upstream unavailable") {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestEditorHandler(t *testing.T) {
	// Verifica que editorHandler no cause panic con upstream inválido
	t.Run("invalid upstream", func(t *testing.T) {
		t.Setenv("EDITOR_UPSTREAM_URL", "://bad")
		defer t.Setenv("EDITOR_UPSTREAM_URL", "")
		fs := server.NewServer()
		s := fs
		editorHandler(s)
	})

	// Verifica que editorHandler proxyee requests y aplica headers esperados.
	t.Run("proxies requests", func(t *testing.T) {
		gotPath := ""
		gotPrefix := ""
		gotForwardedProto := ""
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotPrefix = r.Header.Get("X-Forwarded-Prefix")
			gotForwardedProto = r.Header.Get("X-Forwarded-Proto")
			w.WriteHeader(http.StatusCreated)
		}))
		defer upstream.Close()

		t.Setenv("EDITOR_UPSTREAM_URL", upstream.URL)
		defer t.Setenv("EDITOR_UPSTREAM_URL", "")

		fs := server.NewServer()
		s := fs
		editorHandler(s)

		proxyServer := httptest.NewServer(fs.Mux)
		defer proxyServer.Close()

		req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/editor/assets/app.js", nil)
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		req.Header.Set("Host", "editor.local")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want 201", res.StatusCode)
		}
		if gotPath != "/assets/app.js" {
			t.Fatalf("upstream path = %q", gotPath)
		}
		if gotPrefix != "/editor" {
			t.Fatalf("X-Forwarded-Prefix = %q", gotPrefix)
		}
		if gotForwardedProto != "http" {
			t.Fatalf("X-Forwarded-Proto = %q", gotForwardedProto)
		}
	})

	t.Run("returns bad gateway when upstream is down", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		upstreamURL := upstream.URL
		upstream.Close()

		t.Setenv("EDITOR_UPSTREAM_URL", upstreamURL)

		fs := server.NewServer()
		s := fs
		editorHandler(s)

		proxyServer := httptest.NewServer(fs.Mux)
		defer proxyServer.Close()

		res, err := http.Get(proxyServer.URL + "/editor")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", res.StatusCode)
		}
	})
}
