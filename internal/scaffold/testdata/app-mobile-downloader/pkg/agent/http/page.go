package agent

import (
	"context"
	"net/http"
	"strings"

	"testboi1/internal/shared/mounted"
	"testboi1/internal/shared/server"
	"testboi1/internal/ui/layout"
	agentapp "testboi1/pkg/agent/application"
	agentui "testboi1/pkg/agent/ui"

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

func pageHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler, lookup SessionLookup, oidcCfg OIDCRefreshConfig) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Get(s.Server, "/agent", dashboardPage(manager, lookup, oidcCfg), mw)
	fuego.Get(s.Server, "/agent/home", sessionsPage(lookup, oidcCfg), mw)
	fuego.Get(s.Server, "/agent/providers", providersPage(lookup, oidcCfg), mw)
	fuego.Get(s.Server, "/agent/login", providersPage(lookup, oidcCfg), mw)
}

func dashboardPage(manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state := agentui.PageState{
			ActiveSessionID: strings.TrimSpace(c.QueryParam("session")),
			MountPrefix:     mounted.Prefix(c.Request()),
			DefaultCWD:      ".",
			DefaultModel:    "",
			CurrentView:     "dashboard",
			UserJWT:         resolveUserJWT(c.Context(), c.Request(), lookup, oidcCfg),
		}
		if redirectTo := resolveAgentEntryRedirect(c.Context(), manager, state); redirectTo != "" {
			http.Redirect(c.Response(), c.Request(), redirectTo, http.StatusFound)
			return nil, nil
		}
		return renderDashboardPage(c, state)
	}
}

func sessionsPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state := agentui.PageState{
			MountPrefix:  mounted.Prefix(c.Request()),
			DefaultCWD:   ".",
			DefaultModel: "",
			CurrentView:  "dashboard",
			UserJWT:      resolveUserJWT(c.Context(), c.Request(), lookup, oidcCfg),
		}
		return renderDashboardPage(c, state)
	}
}

func renderDashboardPage(c fuego.ContextNoBody, state agentui.PageState) (any, error) {
	nav := layout.FromRequest(c.Request())
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	return nil, agentui.StandalonePage(state, nav.ActiveThemeID, nav.ThemeCSSHref).Render(c.Context(), c.Response())
}

func resolveAgentEntryRedirect(ctx context.Context, manager agentapp.AgentService, state agentui.PageState) string {
	if manager == nil {
		return ""
	}
	ownerSessionID := previewOwnerSessionIDFromMountPrefix(state.MountPrefix)
	requestedSessionID := strings.TrimSpace(state.ActiveSessionID)
	if requestedSessionID != "" {
		if ownerSessionID != "" && requestedSessionID == ownerSessionID {
			requestedSessionID = ""
		} else if _, err := manager.Get(ctx, requestedSessionID); err == nil {
			return ""
		} else {
			requestedSessionID = ""
		}
	}
	if requestedSessionID != "" {
		return ""
	}
	sessions, err := manager.List(ctx)
	if err != nil || len(sessions) == 0 {
		return agentuiAppPath(state, "/agent/home")
	}
	for _, session := range sessions {
		id := strings.TrimSpace(session.ID)
		if id == "" || (ownerSessionID != "" && id == ownerSessionID) {
			continue
		}
		return agentuiAppPath(state, "/agent?session="+id)
	}
	return agentuiAppPath(state, "/agent/home")
}

func agentuiAppPath(state agentui.PageState, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	return mounted.App(state.MountPrefix, path)
}

func providersPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state := agentui.PageState{
			MountPrefix:  mounted.Prefix(c.Request()),
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
