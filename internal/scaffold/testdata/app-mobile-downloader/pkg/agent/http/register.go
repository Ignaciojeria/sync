// Package http monta la UI del agente y sus endpoints HTTP en el
// server del host.
//
// El modo activo del proyecto es app única (`cmd/api`): la página
// `/agent`, `/agent/auth` y los endpoints de datos
// `/agent/sessions/<id>/{prompt,events,abort,...}` viven en el mismo
// proceso vía RegisterAllLegacy.
//
// Register se conserva para embedders que quieran solo la página del
// agente sin los endpoints de datos; RegisterAllLegacy es el wiring
// normal del proyecto hoy.
package agent

import (
	"net/http"
	"time"

	authapp "testboi1/internal/auth/application"
	authmiddleware "testboi1/internal/auth/middleware"
	"testboi1/internal/shared/server"
	agentapp "testboi1/pkg/agent/application"
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

// Register monta /agent (la página completa) y /agent/auth sin los
// endpoints de datos. Útil solo para embedders que separen UI y runtime.
func Register(s *server.Server, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	requireEditor := authmiddleware.RequireEditor()
	pageHandler(s, nil, requireEditor, lookup, oidcCfg)
	authHandler(s, lookup, oidcCfg)
}

// RegisterAllLegacy monta también los endpoints de datos del agente.
//
// Aunque el nombre quedó histórico, este es el wiring normal del
// proyecto en modo app única (`cmd/api`).
func RegisterAllLegacy(s *server.Server, manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	requireEditor := authmiddleware.RequireEditor()
	pageHandler(s, manager, requireEditor, lookup, oidcCfg)
	authHandler(s, lookup, oidcCfg)
	sessionsHandler(s, manager, requireEditor)
	applyHandler(s, manager, requireEditor)
	mergeHandler(s, manager, requireEditor)
	previewContextHandler(s, manager, requireEditor)
	previewContextUIHandler(s, manager, requireEditor)
	previewControlHandler(s, manager, requireEditor)
	previewProxyHandler(s, manager, requireEditor)
	promptHandler(s, manager, requireEditor)
	abortHandler(s, manager, requireEditor)
	eventsHandler(s, manager, requireEditor)
}

// var _ para que el linter no reporte imports unused en builds sin
// los handlers legacy. (Cuando se llama RegisterAllLegacy, los handlers
// referenciados arriba sí se usan.)
var _ = http.MethodGet
