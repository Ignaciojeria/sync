package agent

import (
	"context"
	agentapp "fixtests1/internal/agent/application"
	agentui "fixtests1/internal/agent/ui"
	"fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
	"fixtests1/internal/ui/layout"
	"net/http"
	"strings"
)

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
	server.Handle(s, "GET /agent", requireEditor(dashboardPage(manager, lookup, oidcCfg)))
	server.Handle(s, "GET /agent/home", requireEditor(sessionsPage(lookup, oidcCfg)))
	server.Handle(s, "GET /agent/providers", requireEditor(providersPage(lookup, oidcCfg)))
	server.Handle(s, "GET /agent/login", requireEditor(providersPage(lookup, oidcCfg)))
}

func dashboardPage(manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := agentui.PageState{
			ActiveSessionID: strings.TrimSpace(r.URL.Query().Get("session")),
			MountPrefix:     mounted.Prefix(r),
			DefaultCWD:      ".",
			DefaultModel:    "",
			CurrentView:     "dashboard",
			UserJWT:         resolveUserJWT(r.Context(), r, lookup, oidcCfg),
		}
		if redirectTo := resolveAgentEntryRedirect(r.Context(), manager, state); redirectTo != "" {
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}
		renderDashboardPage(w, r, state)
	}
}

func sessionsPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := agentui.PageState{
			MountPrefix:  mounted.Prefix(r),
			DefaultCWD:   ".",
			DefaultModel: "",
			CurrentView:  "dashboard",
			UserJWT:      resolveUserJWT(r.Context(), r, lookup, oidcCfg),
		}
		renderDashboardPage(w, r, state)
	}
}

func renderDashboardPage(w http.ResponseWriter, r *http.Request, state agentui.PageState) {
	nav := layout.FromRequest(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := agentui.StandalonePage(state, nav.ActiveThemeID, nav.ThemeCSSHref).Render(r.Context(), w); err != nil {
		writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
	}
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

func providersPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := agentui.PageState{
			MountPrefix:  mounted.Prefix(r),
			DefaultCWD:   ".",
			DefaultModel: "",
			CurrentView:  "providers",
			UserJWT:      resolveUserJWT(r.Context(), r, lookup, oidcCfg),
		}
		nav := layout.FromRequest(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := agentui.StandalonePageWithContent(state, nav.ActiveThemeID, nav.ThemeCSSHref, agentui.ProvidersPage(state)).Render(r.Context(), w); err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
		}
	}
}
