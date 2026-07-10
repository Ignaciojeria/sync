package agent

import (
	"context"
	"net/http"
	"strings"

	agentui "scaffoldxd1/pkg/agent/ui"
	"scaffoldxd1/internal/shared/server"
	"scaffoldxd1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

// resolveUserJWT lee el cookie de sesión del request, lo busca en el
// SessionLookup y devuelve el IDToken crudo del usuario para que el JS
// del browser pueda hablar con el worker vía Authorization: Bearer.
//
// Si el IDToken está vencido o ausente, dispara un refresh contra el
// IdP via grant_type=refresh_token antes de devolver. Si el refresh
// falla (token revocado, refresh_token vacío, red caída, etc.) devuelve
// "" — la UI sigue renderizando y el JS verá 401 al primer prompt.
// El usuario tiene que volver a loguearse en ese caso.
func resolveUserJWT(ctx context.Context, r *http.Request, lookup SessionLookup, oidcCfg OIDCRefreshConfig) string {
	if r == nil || lookup == nil {
		return ""
	}
	sessionID, ok := sessionIDFromCookie(r)
	if !ok {
		return ""
	}
	rec, err := resolveFreshIDToken(ctx, lookup, oidcCfg, sessionID)
	if err != nil || strings.TrimSpace(rec.IDToken) == "" {
		return ""
	}
	return strings.TrimSpace(rec.IDToken)
}

func pageHandler(s *server.Server, requireEditor func(http.Handler) http.Handler, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Get(s.Server, "/agent", dashboardPage(lookup, oidcCfg), mw)
	fuego.Get(s.Server, "/agent/providers", providersPage(lookup, oidcCfg), mw)
	fuego.Get(s.Server, "/agent/login", providersPage(lookup, oidcCfg), mw)
}

func dashboardPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state := agentui.PageState{
			ActiveSessionID: strings.TrimSpace(c.QueryParam("session")),
			DefaultCWD:      ".",
			DefaultModel:    "",
			CurrentView:     "dashboard",
			UserJWT:         resolveUserJWT(c.Context(), c.Request(), lookup, oidcCfg),
		}
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, agentui.StandalonePage(state, nav.ActiveThemeID, nav.ThemeCSSHref).Render(c.Context(), c.Response())
	}
}

func providersPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state := agentui.PageState{
			DefaultCWD:   ".",
			DefaultModel: "",
			CurrentView:  "providers",
			UserJWT:      resolveUserJWT(c.Context(), c.Request(), lookup, oidcCfg),
		}
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, agentui.StandalonePageWithContent(state, nav.ActiveThemeID, nav.ThemeCSSHref, agentui.ProvidersPage(state)).Render(c.Context(), c.Response())
	}
}
