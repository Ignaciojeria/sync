package editor

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)



func editorHandler(s *server.Server) {
	upstream := editorUpstreamURL()

	target, err := url.Parse(upstream)
	if err != nil {
		log.Printf("invalid EDITOR_UPSTREAM_URL %q: %v", upstream, err)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		rewriteEditorRequest(target, originalDirector, r)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		handleEditorProxyError(w, err)
	}
	proxy.ModifyResponse = func(r *http.Response) error {
		stripFrameDenyHeaders(r)
		return nil
	}

	for _, path := range []string{
		"/editor", "/editor/",
		"/assets/",
		"/api/", "/api",
		"/manifest.json",
		"/favicon.ico",
		"/icon.svg",
		"/icon-180.png",
	} {
		fuego.Handle(s.Server, path, proxy)
	}
}

func editorUpstreamURL() string {
	upstream := strings.TrimSpace(os.Getenv("EDITOR_UPSTREAM_URL"))
	if upstream == "" {
		return "http://127.0.0.1:9090"
	}
	return upstream
}

func rewriteEditorRequest(target *url.URL, originalDirector func(*http.Request), r *http.Request) {
	originalDirector(r)
	r.Host = target.Host
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/editor")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
	r.URL.RawPath = r.URL.Path
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Header.Set("X-Forwarded-Proto", forwardedProto(r))
	if prefix := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix")); prefix == "" {
		r.Header.Set("X-Forwarded-Prefix", "/editor")
	}
}

func handleEditorProxyError(w http.ResponseWriter, err error) {
	log.Printf("editor proxy error: %v", err)
	http.Error(w, "editor upstream unavailable", http.StatusBadGateway)
}

func stripFrameDenyHeaders(r *http.Response) {
	// Eliminar headers que impiden embeber el upstream en un iframe
	r.Header.Del("X-Frame-Options")
	r.Header.Del("Content-Security-Policy")
}

func forwardedProto(r *http.Request) string {
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
