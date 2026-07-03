// Package main implementa el agent-worker.
//
// En la Opción A (cada servicio valida JWT contra el IdP), el worker
// corre su propio JWTMiddleware directamente:
//
//   - Carga JWKS_URL al startup (keyfunc refresca las claves de Casdoor
//     en background).
//   - Cada request entrante (con Authorization: Bearer <jwt>) se
//     valida contra ese JWKS, se extrae el email del claim, y se
//     inyecta en el contexto del request para que los handlers
//     decidan permisos.
//
// El worker NO depende del BFF. Si el BFF está caído, el browser puede
// pegarle directamente al worker con el JWT. La diferencia entre
// pasar por el BFF o ir directo es esencialmente 0 para el worker: en
// ambos casos valida el JWT contra el mismo JWKS.
//
// Sin el internal_token HMAC ni secretos compartidos: defensa en
// profundidad vía el estándar OIDC de Casdoor.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/pkg/agent/application"
	agentdisk "app-mobile-downloader/pkg/agent/infrastructure/disk"
	agentmemory "app-mobile-downloader/pkg/agent/infrastructure/memory"
	agentpirpc "app-mobile-downloader/pkg/agent/infrastructure/pirpc"
	workerhandlers "app-mobile-downloader/pkg/agent/worker/handlers"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

func main() {
	listen := os.Getenv("AGENT_WORKER_PORT")
	if listen == "" {
		listen = "127.0.0.1:18080"
	}

	// Defer recover en main: si algo en el setup explota con un
	// panic, loguearlo con stack trace en vez de morir en silencio
	// como antes (el 502 de ayer fue un worker que se cayó sin
	// dejar registro). ¡Mortal: el agente siempre va a loguear la
	// post-mortem!
	defer func() {
		if rcv := recover(); rcv != nil {
			log.Printf("FATAL agent-worker setup panic: %v\n%s", rcv, debug.Stack())
			os.Exit(1)
		}
	}()

	// signal.NotifyContext cancela el contexto cuando llega SIGINT o
	// SIGTERM. Eso nos deja apagar limpio (srv.Shutdown, libera
	// sesiones) en vez de morir abruptamente.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	authMW, err := authMiddleware(ctx)
	if err != nil {
		log.Fatalf("auth middleware: %v", err)
	}

	store, err := loadSessionStore()
	if err != nil {
		log.Fatalf("agent session store: %v", err)
	}

	manager := application.NewManager(store, agentpirpc.NewRunner(""))

	mux := http.NewServeMux()
	registerRoutes(mux, manager, authMW)

	srv := &http.Server{
		Addr: listen,
		// recoverPanic envuelve cada handler para que un panic en una
		// request quede logueado con stack trace, devuelva 500 y el
		// proceso SIGA VIVO en vez de morirse.
		Handler:           recoverPanic(mux),
		ReadHeaderTimeout: 10 * time.Second,
		// WriteTimeout = 0 es intencional: las conexiones SSE del worker
		// son long-lived. Si lo seteáramos, el server cortaría el stream
		// cada X segundos. Cerramos por AbortController del cliente o
		// por shutdown del srv, no por timeout.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second, // cierra conexiones idle (non-SSE).
	}

	srvErrCh := make(chan error, 1)
	go func() {
		log.Printf("agent-worker: listening on %s", listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Printf("agent-worker: signal recibido, apagando limpio")
	case err := <-srvErrCh:
		log.Printf("agent-worker: ListenAndServe falló: %v", err)
	}

	// Shutdown con timeout para no colgar si un cliente SSE no acata.
	shutdownCtx, sCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer sCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("agent-worker: shutdown error: %v", err)
	}
	log.Printf("agent-worker: cerrado")
}

// recoverPanic envuelve un handler para que un panic quede logueado
// con stack trace y devuelva 500 al cliente, sin matar el proceso.
// Aplica a todos los handlers del worker, no sólo a los del agente.
//
// Un panic en una goroutine DE SERVICIO es casi siempre bug — antes
// moríramos en silencio y el BFF devolvía 502 sin contexto, así que
// el operator tenía que adivinar qué reventó. Ahora cualquier panic
// aparece en el log con stack, request path y método.
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rcv := recover(); rcv != nil {
				log.Printf("PANIC handler %s %s remote=%s: %v\n%s",
					r.Method, r.URL.Path, r.RemoteAddr, rcv, debug.Stack())
				// Si el handler no alcanzó a escribir nada, devolvemos
				// un 500 mínimo. Si ya empezó el stream SSE, no podemos
				// cambiar el status; tiramos la conexión (client notará
				// drop y se reconnectará).
				if w.Header().Get("Content-Type") == "" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal error"}`))
				}
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// registerRoutes monta:
//   - GET /agent/healthz SIN auth (es liveness probe; cualquier debe
//     poder preguntarlo sin timestep.
//   - GET /agent/sessions, POST /agent/sessions, y sub-rutas
//     /agent/sessions/<id>/{prompt,steer,abort,events,get} CON auth.
//
// Nota: "/agent" exacto NO vive acá. Es la UI (templ.Page) que sirve el
// web-server; el BFF la deja pasar al web y sólo enruta al worker lo
// que cae bajo "/agent/<algo>". Ver cmd/bff/main.go:isAgentRoute.
func registerRoutes(mux *http.ServeMux, mgr application.AgentService, authMW func(http.Handler) http.Handler) {
	mux.HandleFunc("/agent/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"alive"}`)
	})

	workerhandlers.Register(mux, mgr, authMW)
}

// authMiddleware construye el JWTMiddleware del worker. Usa el mismo
// conf que el web-server (issuer, audience) más el JWKS cargado
// desde JWKS_URL (o HMAC si JWT_HMAC_SECRET está configurado, para dev).
//
// El worker NO usa sessionStore (no hay cookies / login flow): los
// browsers pasan JWT bearer directamente. Por eso pasamos nil a
// JWTMiddleware, que saltea el bloque de cookies y va directo a JWT.
func authMiddleware(ctx context.Context) (func(http.Handler) http.Handler, error) {
	conf := configuration.Conf{
		OIDCIssuer:   os.Getenv("OIDC_ISSUER"),
		OIDCClientID: os.Getenv("OIDC_CLIENT_ID"),
		JWTAudience:  os.Getenv("JWT_AUDIENCE"),
	}

	kf, err := loadKeyfunc(ctx)
	if err != nil {
		return nil, fmt.Errorf("load keyfunc: %w", err)
	}
	mw := authmiddleware.JWTMiddleware(kf, nil, conf)
	return mw, nil
}

// hmacKeyfunc implementa keyfunc.Keyfunc validando JWTs firmados con
// HMAC contra un shared secret. Sólo se usa en dev/CI; en prod el
// worker usa JWKS_URL.
type hmacKeyfunc struct{ secret []byte }

func (h hmacKeyfunc) Keyfunc(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}
	return h.secret, nil
}

// KeyfuncCtx devuelve un jwt.Keyfunc estándar (sin context).
func (h hmacKeyfunc) KeyfuncCtx(_ context.Context) jwt.Keyfunc {
	return h.Keyfunc
}

// Storage devuelve nil (no hay storage de JWKS).
func (h hmacKeyfunc) Storage() jwkset.Storage {
	return nil
}

// VerificationKeySet devuelve un error: HMAC mode no tiene JWK Set; el
// path de validación vía keyfunc+jwt no se usa aquí.
func (h hmacKeyfunc) VerificationKeySet(_ context.Context) (jwt.VerificationKeySet, error) {
	return jwt.VerificationKeySet{}, fmt.Errorf("HMAC mode no tiene VerificationKeySet; usa Keyfunc() directamente")
}

// loadKeyfunc carga JWKS_URL (producción) o usa HMAC secret (dev/CI).
// Para HMAC, keyfunc.NewDefault no aplica (sólo JWKS); construimos un
// adapter con hmacKeyfunc que satisface la misma interfaz.
func loadKeyfunc(ctx context.Context) (keyfunc.Keyfunc, error) {
	jwksURL := strings.TrimSpace(os.Getenv("JWKS_URL"))
	if jwksURL != "" {
		kf, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
		if err != nil {
			return nil, fmt.Errorf("JWKS %q: %w", jwksURL, err)
		}
		return kf, nil
	}
	hmacSecret := os.Getenv("JWT_HMAC_SECRET")
	if hmacSecret == "" {
		return nil, fmt.Errorf("ni JWKS_URL ni JWT_HMAC_SECRET configurados")
	}
	log.Println("agent-worker: HMAC secret mode (NO usar en prod)")
	return hmacKeyfunc{secret: []byte(hmacSecret)}, nil
}

// loadSessionStore elige el backing store. Disco bajo
// $AGENT_SESSION_DIR o ./tmp/agent-sessions, fallback a memoria si el
// disco no inicializa.
func loadSessionStore() (application.SessionStore, error) {
	dir := strings.TrimSpace(os.Getenv("AGENT_SESSION_DIR"))
	if dir == "" {
		dir = "tmp/agent-sessions"
	}
	store, err := agentdisk.NewSessionStore(dir)
	if err == nil {
		log.Printf("agent-worker session store: disk at %s", store.Dir())
		return store, nil
	}
	log.Printf("agent-worker session store: disk unavailable (%v); falling back to memory", err)
	return agentmemory.NewSessionStore(), nil
}
