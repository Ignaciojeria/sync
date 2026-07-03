package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestIsAgentRoute cubre la frontera que define qué va al worker y qué
// va al web-server.
//
//   - "/agent" exacto es la UI (templ.Page) en el web → false.
//   - "/agent/auth" también vive en el web (resuelve + refresca JWT) → false.
//   - "/agent/<x>" es la API del worker → true.
//   - "/agents"/plural NO va al worker → false.
func TestIsAgentRoute(t *testing.T) {
	cases := map[string]bool{
		"/agent":                   false, // UI en el web, no worker
		"/agent/auth":              false, // endpoint JWT en el web, no worker
		"/agent/":                  true,
		"/agent/healthz":           true,
		"/agent/sessions/x/prompt": true,
		"/":                        false,
		"/agents":                  false, // plural: NO va al worker
		"/agents/foo":              false,
		"/editor":                  false,
		"/design/theme":            false,
		"/somewhere":               false,
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			if got := isAgentRoute(path); got != want {
				t.Errorf("isAgentRoute(%q) = %v, want %v", path, got, want)
			}
		})
	}
}

// TestBFFRouteWebAndAgent monta dos upstreams de prueba (web y agent)
// y verifica que el BFF rutea correctamente:
//
//	/agent/*     → agent
//	*            → web
//
// Observación: como httputil.NewSingleHostReverseProxy usa el Host del
// upstream en la URL del request, los servers de prueba reciben el
// prefijo `/agent` correctamente.
func TestBFFRouteWebAndAgent(t *testing.T) {
	webSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "web:"+r.URL.Path)
	}))
	defer webSrv.Close()

	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "agent:"+r.URL.Path)
	}))
	defer agentSrv.Close()

	handler := newTestMux(mustParseURL(t, webSrv.URL), mustParseURL(t, agentSrv.URL))

	cases := map[string]string{
		"/agent/healthz":           "agent:/agent/healthz",
		"/agent/sessions/x/prompt": "agent:/agent/sessions/x/prompt",
		"/":                        "web:/",
		"/editor":                  "web:/editor",
		"/design":                  "web:/design",
	}
	for path, want := range cases {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := strings.TrimSpace(rec.Body.String())
			if got != want {
				t.Errorf("path %q → %q, want %q", path, got, want)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status para %q = %d, want 200", path, rec.Code)
			}
		})
	}
}

// TestBFFReturns502WhenAgentDown verifica que cuando el upstream del
// agent no responde, NewSingleHostReverseProxy devuelve un error
// transport-level (502) en lugar de tomar la ruta web como fallback.
//
// Esto es importante: si /agent/* falla en el worker, el BFF NO
// debe derivarlo al web-server (que tampoco sabe del agente). El
// cliente debe ver 502 y reintentar para que el prototipo de hot
// reload deje claro qué capa está fallando.
func TestBFFReturns502WhenAgentDown(t *testing.T) {
	// Levantamos un agent upstream y lo bajamos antes del request.
	agentSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	agentURL := mustParseURL(t, agentSrv.URL)
	agentSrv.Close() // <- clave: lo bajamos para que NewSingleHostReverseProxy falle
	webSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("el BFF no debería rutear /agent/* al web-server cuando el agent upstream está caído")
	}))
	defer webSrv.Close()
	webURL := mustParseURL(t, webSrv.URL)

	handler := newTestMux(webURL, agentURL)

	req := httptest.NewRequest(http.MethodGet, "/agent/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 502 Bad Gateway es el estándar cuando un upstream reverse-proxy
	// no responde. Si el BFF cambió ese comportamiento, este test
	// falla y obliga a revisar la decisión.
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d (502)", rec.Code, http.StatusBadGateway)
	}
}

// TestBFFPreservesAuthorizationHeader verifica que el BFF (Opción A:
// dumb proxy) PRESERVA el header Authorization cuando forwardea, sin
// tocarlo. Esto es crítico: cada upstream valida JWT contra Casdoor,
// así que el JWT debe pasar sin modificaciones del BFF.
func TestBFFPreservesAuthorizationHeader(t *testing.T) {
	var receivedAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	handler := newTestMux(mustParseURL(t, upstream.URL), mustParseURL(t, upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	req.Header.Set("Authorization", "Bearer jwt-from-browser-unchanged")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	want := "Bearer jwt-from-browser-unchanged"
	if receivedAuth != want {
		t.Errorf("upstream recibió Authorization = %q, want %q (BFF debe pasar el header tal cual)", receivedAuth, want)
	}
}

// newTestMux extrae el armado del mux principal para que los tests
// puedan usarlo sin involucrar ListenAndServe. Es un espejo del main()
// pero sin el wiring de env vars ni log.Fatalf.
func newTestMux(webURL, agentURL *url.URL) http.Handler {
	webProxy := newProxy(webURL)
	agentProxy := newProxy(agentURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isAgentRoute(r.URL.Path) {
			agentProxy.ServeHTTP(w, r)
			return
		}
		webProxy.ServeHTTP(w, r)
	})
	return mux
}

// helpers

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
