package agent

import (
	"context"
	agentapp "lastmile-agents/internal/agent/application"
	agentuiv2 "lastmile-agents/internal/agent/ui/v2"
	gatewayapp "lastmile-agents/internal/gateway/application"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"lastmile-agents/internal/shared/mounted"
	"lastmile-agents/internal/shared/server"
	"lastmile-agents/internal/ui/layout"
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

// pageHandler monta la página y los assets estáticos del cliente
// V2 en /agent/*. NO monta los endpoints de datos: eso lo hace
// `Register` llamando a sessionsHandler/promptHandler/etc. directo
// (los data handlers vivían en /agent-v2 con forwarders al V1
// durante la fase de construcción; tras el cutover se sirven
// directo desde /agent, así que la separación page/data sigue
// siendo la misma que tenía V1).
//
// Antes (pre-cutover 2026-07) había dos page handlers separados:
// pageHandler (V1, vivía en /agent) y pageV2Handler (V2, vivía
// en /agent-v2). El cutover los fusionó: la V1 dejó de existir
// y la V2 pasó a /agent. La firma requiere `sessionCostSvc`
// (no nil en prod) para hacer el pre-render del budget bar.
// En tests se pasa nil y el page handler omite ese fetch.
func pageHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler, lookup SessionLookup, oidcCfg OIDCRefreshConfig, sessionCostSvc *gatewayapp.SessionCostService) {
	// Páginas.
	server.Handle(s, "GET /agent", requireEditor(dashboardPage(manager, lookup, oidcCfg, sessionCostSvc)))
	server.Handle(s, "GET /agent/", requireEditor(canonicalAgentEntry()))
	server.Handle(s, "GET /agent/home", requireEditor(sessionsPage(lookup, oidcCfg)))
	// Endpoint V2-only: regenerate re-envía el último prompt del
	// user borrando las respuestas del assistant previas. Lo monta
	// `pageHandler` y NO `Register` porque es UI-shell (V2-only) y
	// no convivía con el wiring V1.
	server.Handle(s, "POST /agent/sessions/{id}/regenerate", requireEditor(regenerateHandler(manager)))
	// Assets estáticos del módulo JS V2. Se sirven desde
	// internal/agent/ui/v2/static/agent-chat/ via embed.FS. Sin
	// auth (no leen nada sensible: son JS público del shell).
	server.Handle(s, "GET /agent/static/", serveAgentStatic())
}

func dashboardPage(manager agentapp.AgentService, lookup SessionLookup, oidcCfg OIDCRefreshConfig, sessionCostSvc *gatewayapp.SessionCostService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := agentuiv2.PageState{
			ActiveSessionID: strings.TrimSpace(r.URL.Query().Get("session")),
			MountPrefix:     mounted.Prefix(r),
			DefaultCWD:      ".",
			DefaultModel:    "",
			CurrentView:     "dashboard",
			UserJWT:         resolveUserJWT(r.Context(), r, lookup, oidcCfg),
		}
		if redirectTo := resolveAgentEntryRedirect(r.Context(), manager, state.MountPrefix, state.ActiveSessionID, "/agent"); redirectTo != "" {
			http.Redirect(w, r, redirectTo, http.StatusFound)
			return
		}
		if manager != nil {
			if sessions, err := manager.List(r.Context()); err == nil {
				state.Sessions = sessions
			}
			if state.ActiveSessionID != "" {
				if active, err := manager.Get(r.Context(), state.ActiveSessionID); err == nil {
					state.ActiveSession = &active
				}
				if history, err := agentapp.LoadConversationHistoryCtx(r.Context(), state.ActiveSessionID, 0, 30); err == nil {
					state.ConversationItems = history.Items
				}
				// ponytail: registramos el renderer V2 ANTES de
				// renderizar la página. Si el browser abre el SSE
				// inmediatamente después, el lookup por sessionID ya
				// tiene la entrada y emite HTML V2. Sin este orden,
				// los primeros events llegarían con HTML V1 (el
				// fallback) hasta que el handler llamara Set.
				agentuiv2.RegisterRendererForSession(state.ActiveSessionID)
				// ponytail: pre-render del budget bar (cost + tokens +
				// modelo) llamando al gateway al render. Sin este
				// fetch inicial, la barra muestra "—" en el primer
				// paint hasta que el cliente haga su primer poll (15s).
				// El snapshot server-rendered evita el parpadeo.
				if sessionCostSvc != nil {
					if resp, err := sessionCostSvc.Fetch(r.Context(), state.ActiveSessionID); err == nil {
						state.SessionCostUSD = resp.EstimatedCostUSD
						state.SessionCostReady = true
						state.SessionCostReqs = resp.RequestCount
						state.SessionModelAlias = resp.ModelAlias
					state.SessionContextWindow = resp.ContextWindow
					state.SessionPromptTokens = resp.PromptTokens
					state.SessionCompletionTokens = resp.CompletionTokens
					state.SessionCurrentPromptTokens = resp.CurrentPromptTokens
					state.SessionTotalTokens = resp.TotalTokens
					}
				}
			}
		}
		renderAgentDashboardPage(w, r, state)
	}
}

func sessionsPage(lookup SessionLookup, oidcCfg OIDCRefreshConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := agentuiv2.PageState{
			MountPrefix:  mounted.Prefix(r),
			DefaultCWD:   ".",
			DefaultModel: "",
			CurrentView:  "dashboard",
			UserJWT:      resolveUserJWT(r.Context(), r, lookup, oidcCfg),
		}
		renderAgentDashboardPage(w, r, state)
	}
}

func canonicalAgentEntry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		location := agentuiAppPathFromPrefix(mounted.Prefix(r), "/agent")
		if q := strings.TrimSpace(r.URL.RawQuery); q != "" {
			location += "?" + q
		}
		http.Redirect(w, r, location, http.StatusMovedPermanently)
	}
}

func renderAgentDashboardPage(w http.ResponseWriter, r *http.Request, state agentuiv2.PageState) {
	nav := layout.FromRequest(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := agentuiv2.StandalonePage(state, nav.ActiveThemeID, nav.ThemeCSSHref).Render(r.Context(), w); err != nil {
		writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
	}
}

// serveAgentStatic sirve el módulo JS del chat desde el embed.FS
// del paquete ui/v2. La ruta /agent/static/<archivo> mapea a
// static/agent-chat/<archivo> dentro del FS.
//
// MIME type se deduce de la extensión. Cache-Control "no-store"
// durante M-A para que el browser siempre pida la versión actual
// y un fix del state machine del cliente no quede invisible por
// cache intermedia. En prod se puede volver a un cache más
// agresivo con hashes en los filenames.
func serveAgentStatic() http.Handler {
	sub, err := fs.Sub(agentuiv2.AssetsFS, "static/agent-chat")
	if err != nil {
		// embed.FS debería siempre tener la carpeta; si falla es un
		// bug de build. Devolvemos un handler que siempre 500.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "agent static FS not embedded", http.StatusInternalServerError)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.StripPrefix("/agent/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defensa contra path traversal: limpiar y limitar a la
		// raíz del FS sub.
		cleaned := path.Clean(r.URL.Path)
		if strings.Contains(cleaned, "..") || strings.HasPrefix(cleaned, "/") {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = cleaned
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	}))
}

// regenerateHandler borra las respuestas del assistant desde el
// último user prompt y re-envía el prompt. El cliente recibe el
// clearAfterSeq en el body, borra los items del feed con seq
// mayor a ese, y luego los nuevos fragments del assistant (con
// seqs nuevos) se renderizan normalmente.
//
// Limitaciones (M-C.2 simple):
// - Solo funciona para el último prompt del user.
// - Los side effects de tools ejecutadas en el turno viejo
//   no se rollbackean.
func regenerateHandler(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		if !isSafeSessionID(sessionID) {
			writeError(w, server.HTTPError{Status: http.StatusBadRequest, Detail: "invalid session id"})
			return
		}
		// Encontramos el seq del último user prompt ANTES
		// de regenerar, para devolverlo al cliente.
		items, err := agentapp.LoadConversationHistoryCtx(r.Context(), sessionID, 0, 200)
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		var lastUserSeq uint64
		for i := len(items.Items) - 1; i >= 0; i-- {
			if items.Items[i].Kind == "user" && items.Items[i].Text != "" && items.Items[i].Seq > 0 {
				lastUserSeq = items.Items[i].Seq
				break
			}
		}
		if lastUserSeq == 0 {
			writeError(w, server.HTTPError{
				Status:  http.StatusBadRequest,
				Detail:  "no hay user prompt para regenerar",
			})
			return
		}
		if err := manager.Regenerate(r.Context(), sessionID); err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"clearAfterSeq": lastUserSeq,
		})
	}
}

// resolveAgentEntryRedirect decide si el GET /agent debe
// redirigir a una sesión existente (la primera no-owner) o a la
// lista vacía (/agent/home). Misma heurística que tenía V1; la
// trajimos acá porque /agent/home también es V2 post-cutover.
func resolveAgentEntryRedirect(ctx context.Context, manager agentapp.AgentService, mountPrefix, activeSessionID, basePath string) string {
	if manager == nil {
		return ""
	}
	basePath = normalizeBasePath(basePath)
	ownerSessionID := previewOwnerSessionIDFromMountPrefix(mountPrefix)
	requestedSessionID := strings.TrimSpace(activeSessionID)
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
		return agentuiAppPathFromPrefix(mountPrefix, basePath+"/home")
	}
	for _, session := range sessions {
		id := strings.TrimSpace(session.ID)
		if id == "" || (ownerSessionID != "" && id == ownerSessionID) {
			continue
		}
		return agentuiAppPathFromPrefix(mountPrefix, basePath+"?session="+id)
	}
	return agentuiAppPathFromPrefix(mountPrefix, basePath+"/home")
}

// agentuiAppPathFromPrefix aplica el mount prefix a un path. Es
// una versión reducida de mounted.App que vive acá para no
// acoplar el helper de redirect a un PageState concreto (V1 o V2).
func agentuiAppPathFromPrefix(mountPrefix, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	prefix := strings.TrimSpace(mountPrefix)
	if prefix == "" {
		return path
	}
	prefix = strings.TrimRight(prefix, "/")
	if path == "/" {
		return prefix + "/"
	}
	return prefix + path
}

// normalizeBasePath garantiza que basePath empiece con "/" y no
// termine con "/". El redirect concatena con "/home" o "?session=..."
// y un mal basePath rompe la URL.
func normalizeBasePath(basePath string) string {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		basePath = "/agent"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	return strings.TrimRight(basePath, "/")
}