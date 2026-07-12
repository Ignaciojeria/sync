package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

const previewProxyHeader = "X-Agent-Preview-Proxy"

func previewProxyHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	_ = requireEditor
	handler := http.HandlerFunc(previewProxy(manager))
	fuego.Handle(s.Server, "/agent/sessions/{id}/preview", handler)
	fuego.Handle(s.Server, "/agent/sessions/{id}/preview/{rest...}", handler)
}

func previewProxy(manager agentapp.AgentService) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.Header.Get(previewProxyHeader)) != "" {
			http.Error(w, "preview proxy loop detected", http.StatusLoopDetected)
			return
		}

		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		session, err := manager.Get(r.Context(), id)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, agentapp.ErrSessionNotFound) {
				status = http.StatusNotFound
			}
			http.Error(w, err.Error(), status)
			return
		}
		if session.PreviewPort < 1 {
			http.Error(w, "preview not configured", http.StatusNotFound)
			return
		}
		if session.PreviewStatus != "" && session.PreviewStatus != agentapp.PreviewStatusLive {
			http.Error(w, "preview not live", http.StatusConflict)
			return
		}

		target, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(session.PreviewPort))
		if err != nil {
			http.Error(w, "invalid preview target", http.StatusBadGateway)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		prefix := previewPrefix(id)
		proxy.Director = func(req *http.Request) {
			rest := r.PathValue("rest")
			if rest == "" {
				rest = "/"
			} else {
				rest = "/" + strings.TrimPrefix(rest, "/")
			}
			originalDirector(req)
			req.Host = target.Host
			req.URL.Path = rest
			req.URL.RawPath = rest
			req.Header.Set(previewProxyHeader, "1")
			req.Header.Set("X-Forwarded-Host", r.Host)
			req.Header.Set("X-Forwarded-Proto", forwardedProto(r))
			req.Header.Set("X-Forwarded-Prefix", prefix)
		}
		proxy.ModifyResponse = func(resp *http.Response) error {
			location := strings.TrimSpace(resp.Header.Get("Location"))
			if location == "" || !strings.HasPrefix(location, "/") || strings.HasPrefix(location, "//") {
				return nil
			}
			if strings.HasPrefix(location, prefix) {
				return nil
			}
			resp.Header.Set("Location", strings.TrimRight(prefix, "/")+location)
			return nil
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
			http.Error(w, fmt.Sprintf("preview upstream unavailable: %v", err), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	}
}

func previewPrefix(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "/agent/sessions/" + id + "/preview/"
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
