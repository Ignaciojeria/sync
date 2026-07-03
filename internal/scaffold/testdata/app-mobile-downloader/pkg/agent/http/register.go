// Package http conserva el page handler del agente y lo monta en el
// server del host.
//
// A partir de la migración a 3 procesos (BFF + web-server + agent-worker),
// el web-server SÓLO expone /agent (página completa). Los endpoints de
// datos (/agent/sessions/<id>/prompt, /events, /abort, etc.) viven en
// el agent-worker via pkg/agent/worker/handlers.Register.
//
// El handler de página mira el cookie de sesión (app_session_id), lo
// resuelve contra el SessionLookup provisto, y embebe el IDToken del
// usuario en el HTML para que el JS del browser pueda hablar con el
// worker vía Authorization: Bearer (que es el único idioma que el
// worker entiende). Ver doc/bff-jwt-invariants.md.
//
// Si el proyecto derivado decide NO separar el agente en un binario
// distinto, basta con: (a) usar AGENT_ENABLED=false en el web-server
// o (b) re-registrar los handlers aquí. Ver RegisterAllLegacy abajo.
package agent

import (
	"net/http"
	"time"

	authapp "app-mobile-downloader/internal/auth/application"
	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	agentapp "app-mobile-downloader/pkg/agent/application"
	"app-mobile-downloader/internal/shared/server"
)

// SessionLookup expone lo mínimo del session store para que la UI del
// agente pueda resolver el IDToken del usuario a partir del cookie
// app_session_id y refrescarlo cuando vence. En este proyecto lo
// satisface *authpostgresql.SessionRepository; los tests pueden usar
// un stub.
type SessionLookup interface {
	FindActiveSessionByID(sessionID string) (authapp.Session, error)
	UpdateSessionTokens(sessionID, accessToken, refreshToken, idToken string, expiresAt *time.Time) error
}

// OIDCRefreshConfig tiene lo mínimo para refrescar tokens contra el
// IdP via grant_type=refresh_token. Se inyecta desde cmd/api.
type OIDCRefreshConfig struct {
	TokenEndpoint string
	ClientID      string
	ClientSecret  string
}

// Register monta /agent (la página completa) y /agent/auth en el
// web-server. A diferencia del wiring legacy, NO recibe AgentService:
// la página se hidrata pegándole al worker vía BFF, así que el
// web-server no necesita crear Manager ni levantar pi.
func Register(s *server.Server, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	requireEditor := authmiddleware.RequireEditor()
	pageHandler(s, requireEditor, lookup, oidcCfg)
	authHandler(s, lookup, oidcCfg)
}

// RegisterAllLegacy monta TODOS los endpoints de datos también, para
// cuando el proyecto derivado corre un solo binario (sin BFF ni
// agent-worker separados). Útil para tests E2E y dev simple.
//
// En la topología canónica (3 procesos) NO usar este RegisterAllLegacy:
// los endpoints de datos viven en el worker y se sirven vía BFF. Si
// quedan acá y en el worker, hay duplicación y los handlers compiten
// por las rutas.
func RegisterAllLegacy(s *server.Server, manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	requireEditor := authmiddleware.RequireEditor()
	pageHandler(s, requireEditor, lookup, oidcCfg)
	authHandler(s, lookup, oidcCfg)
	sessionsHandler(s, manager, requireEditor)
	promptHandler(s, manager, requireEditor)
	abortHandler(s, manager, requireEditor)
	eventsHandler(s, manager, requireEditor)
}

// var _ para que el linter no reporte imports unused en builds sin
// los handlers legacy. (Cuando se llama RegisterAllLegacy, los handlers
// referenciados arriba sí se usan.)
var _ = http.MethodGet
