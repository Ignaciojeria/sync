// Package main implementa el BFF (Backend For Frontend) proxy.
//
// El BFF es la "puerta de entrada estable" del sistema. Es un proxy
// HTTP inverso TONTO que rutea:
//
//   /agent/*   →  agent-worker (cuando responde; 502 cuando está caído)
//   *          →  web-server
//
// En esta versión (Opción A: cada servicio valida JWT contra el
// IdP directamente), el BFF NO hace nada con la autenticación:
//
//   - Browser → BFF → upstream, preservando el header `Authorization`.
//   - Cada upstream (web-server, agent-worker) corre su propio
//     JWTMiddleware contra Casdoor (mismas env JWKS_URL, OIDC_ISSUER,
//     etc.) y decide independientemente.
//
// El BFF no añade latencia, no depende del IdP, no introduce secretos
// compartidos, y desaparece del boot-path de auth. Si el BFF se cae,
// los upstreams siguen sirviendo requests con JWT válidos.
//
// El BFF NO está bajo el watch de air: cambios en sus archivos NO
// disparan hot-reload. Para regenerarlo, `go build -o ./bin/bff
// ./cmd/bff` y reiniciar el proceso manualmente.
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

func main() {
	listen := normalizeListenAddr(getenv("BFF_PORT", "8000"))
	webURL := parseURL(getenv("BFF_WEB_UPSTREAM", "http://127.0.0.1:8001"))
	agentURL := parseURL(getenv("BFF_AGENT_UPSTREAM", "http://127.0.0.1:18080"))

	webProxy := newProxy(webURL)
	agentProxy := newProxy(agentURL)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if isAgentRoute(r.URL.Path) {
			log.Printf("bff → agent %s", r.URL.Path)
			agentProxy.ServeHTTP(w, r)
			return
		}
		log.Printf("bff → web   %s", r.URL.Path)
		webProxy.ServeHTTP(w, r)
	})

	log.Printf("bff: listening %s | web=%s agent=%s", listen, webURL, agentURL)
	log.Fatal(http.ListenAndServe(listen, mux))
}

// isAgentRoute devuelve true si el path cae bajo el API del worker.
//
// En la topología de 3 procesos (BFF + web-server + agent-worker):
//   - "/agent" exacto es la UI (templ.Page) que vive en el web-server.
//   - "/agent/auth" también vive en el web-server: es el endpoint que
//     resuelve el IDToken del usuario a partir del cookie + los refresca
//     contra el IdP cuando están vencidos. La UI del agente lo llama
//     antes de cada prompt y reconexión SSE, por eso no puede ir al
//     worker (que no acepta cookie).
//   - "/agent/<resto>" es la API que vive en el worker (sessions,
//     prompt, events, healthz, etc).
//
// Por eso la frontera acá es: cualquier cosa que arranque con "/agent/"
// va al worker — excepto "/agent/auth", que va al web junto con
// "/agent" pelado, "/agents" plural y todo lo que no sea prefijo.
func isAgentRoute(path string) bool {
	if path == "/agent/auth" {
		return false
	}
	return strings.HasPrefix(path, "/agent/")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// normalizeListenAddr acepta tanto "8000" como ":8000" y devuelve
// ":8000". Usar el puerto pelado como default hace más fácil la
// configuración para el operador (no hay que recordar el ":").
func normalizeListenAddr(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.Contains(addr, ":") {
		return addr
	}
	return ":" + addr
}

func parseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		log.Fatalf("bff: parse upstream %q: %v", raw, err)
	}
	return u
}

// newProxy construye el reverse-proxy hacia un upstream. Cuando el
// upstream está caído, NewSingleHostReverseProxy devuelve 502 Bad
// Gateway automáticamente.
func newProxy(target *url.URL) *httputil.ReverseProxy {
	return httputil.NewSingleHostReverseProxy(target)
}
