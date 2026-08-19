// Package agent monta el chat del agente y sus endpoints HTTP en
// el server del host.
//
// El modo activo del proyecto es app única (`cmd/api`): la página
// `/agent`, los endpoints de datos `/agent/sessions/<id>/{prompt,
// events, abort, merge, regenerate, ...}`, el inspector del
// worktree y los assets estáticos del cliente JS viven en el
// mismo proceso vía `Register`.
//
// Antes del cutover de 2026-07 coexistían dos shells en paralelo
// (`RegisterAllLegacy` para V1 en `/agent` y `RegisterV2` para
// la shell nueva en `/agent-v2`). El cutover los fusionó en una
// sola `Register` que vive en `/agent`. Los wrappers deprecated
// se removieron en el mismo commit.
package agent

import (
	"net/http"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
	agentworktree "lastmile-agents/internal/agent/infrastructure/worktree"
	authapp "lastmile-agents/internal/auth/application"
	gatewayapp "lastmile-agents/internal/gateway/application"
	"lastmile-agents/internal/shared/server"
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

// Register monta toda la superficie del agente en `/agent/*`: la
// página, los endpoints de datos de sesión, los assets estáticos
// del cliente JS, el worktree inspector y el endpoint
// `/regenerate`.
//
// requireEditor se inyecta desde el caller (`cmd/api/main.go`)
// usando `authmiddleware.RequireEditor()`. Los tests pasan un
// noop para evitar la dependencia del stack completo de auth
// (JWT + JWKS + DB).
//
// sessionCostSvc se inyecta desde el caller para alimentar el
// pre-render del budget bar. Si es nil, el page handler omite
// ese fetch (modo test).
func Register(s *server.Server, manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig, requireEditor func(http.Handler) http.Handler, sessionCostSvc *gatewayapp.SessionCostService) {
	pageHandler(s, manager, requireEditor, lookup, oidcCfg, sessionCostSvc)
	authHandler(s, lookup, oidcCfg)
	sessionsHandler(s, manager, requireEditor)
	mergeHandler(s, manager, requireEditor)
	previewContextHandler(s, manager, requireEditor)
	previewContextUIHandler(s, manager, requireEditor)
	previewControlHandler(s, manager, requireEditor)
	previewProxyHandler(s, manager, requireEditor)
	promptHandler(s, manager, requireEditor)
	abortHandler(s, manager, requireEditor)
	eventsHandler(s, manager, requireEditor)
	worktreeInspectorHandler(s, manager, agentworktree.NewInspector(), requireEditor)
}

// var _ para que el linter no reporte imports unused si en algún
// build los handlers referenciados arriba no se usan. Hoy siempre
// se llaman desde Register, así que es solo defensa.
var _ = http.MethodGet
