package v2

import (
	"strings"

	agentapp "lastmile-agents/internal/agent/application"
)

// PageState es la metadata que el handler de V2 pasa al template.
// Mismo shape que la V1 (agentui.PageState) pero en este paquete
// propio para que la V2 pueda divergir sin tocar la V1.
//
// Decisión consciente: NO embebemos agentui.PageState. Si mañana la
// V2 necesita un campo que V1 no tiene (ej. un modo "compact" del
// prompt), lo agregamos acá sin afectar V1. Mantener V1/V2
// acopladas por embedding fue el primer error que cometimos cuando
// pensé este refactor — corregido a tiempo.
type PageState struct {
	Sessions      []agentapp.Session
	ActiveSession *agentapp.Session
	MountPrefix   string
	// ActiveSessionID permite renderizar la shell del chat sin
	// depender de la metadata completa de la sesión. Se usa en el
	// redirect heuristic y para que el JS V2 pueda hablar con el
	// backend sin tener que parsear el header.
	ActiveSessionID string
	DefaultCWD      string
	DefaultModel    string
	CurrentView     string
	UserJWT         string
	// ConversationItems es la lista de items pre-renderizados del
	// feed inicial. El JS solo agrega los nuevos que llegan por SSE.
	ConversationItems []agentapp.ConversationItem
	// SessionCost + Balance: snapshot del estado del gateway al
	// momento de cargar la página. Cero round-trips del cliente.
	SessionCostUSD   float64
	SessionCostReady bool
	SessionCostReqs  int
	BalanceUSD       *float64
	// ponytail: contexto del modelo activo. SessionModelAlias es
	// el nombre del modelo (ej. "minimax/m3"); SessionContextWindow
	// es el límite máximo de tokens (ej. 1000000). Se usan para
	// mostrar un badge del modelo y un indicador de uso del
	// contexto (% del context window consumido). Se popula desde
	// el response del gateway al renderizar la página; el cliente
	// los refresca cada 15s junto con el session-cost.
	SessionModelAlias       string
	SessionContextWindow    int64
	SessionPromptTokens     int64
	SessionCompletionTokens int64
	// ponytail: SessionCurrentPromptTokens es el size del
	// context de la ÚLTIMA llamada a la API (no acumulado).
	// Viene del gateway de sync-ai-gateway vía
	// SessionCostService; si el gateway no lo devuelve, se
	// usa el promedio de la sesión como fallback. La UI V2
	// muestra el % de uso del context window basándose en
	// este campo, no en el total acumulado.
	SessionCurrentPromptTokens int64
	SessionTotalTokens         int64
}

func activeSessionID(state PageState) string {
	if strings.TrimSpace(state.ActiveSessionID) != "" {
		return state.ActiveSessionID
	}
	if state.ActiveSession == nil {
		return ""
	}
	return state.ActiveSession.ID
}

// ponytail: RequestCountProxy expone el número de requests de
// la sesión al templ, para que pueda calcular el fallback del
// % del context (promedio de prompt_tokens / request_count)
// cuando el gateway no devuelve current_prompt_tokens. La UI
// ya tiene un campo SessionCostReqs en el state que viene del
// gateway; lo exponemos vía este helper para que el templ no
// tenga que conocer el nombre del campo del gateway.
func (s PageState) RequestCountProxy() int {
	return s.SessionCostReqs
}

func hasConversation(state PageState) bool {
	return len(state.ConversationItems) > 0
}

func shellTitle(state PageState) string {
	if state.ActiveSession == nil || strings.TrimSpace(state.ActiveSession.Title) == "" {
		return "sync4.run · v2"
	}
	return state.ActiveSession.Title
}

func shellModel(state PageState) string {
	if state.ActiveSession != nil && strings.TrimSpace(state.ActiveSession.Model) != "" {
		return state.ActiveSession.Model
	}
	if strings.TrimSpace(state.DefaultModel) != "" {
		return state.DefaultModel
	}
	return "default"
}

func shellDir(state PageState) string {
	if state.ActiveSession != nil && strings.TrimSpace(state.ActiveSession.CWD) != "" {
		return state.ActiveSession.CWD
	}
	if strings.TrimSpace(state.DefaultCWD) != "" {
		return state.DefaultCWD
	}
	return "."
}

// appPath resuelve una ruta de la app respetando el mount prefix
// (preview mode). Misma semántica que la versión V1.
func appPath(state PageState, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	prefix := strings.TrimSpace(state.MountPrefix)
	if prefix == "" {
		return path
	}
	prefix = strings.TrimRight(prefix, "/")
	if path == "/" {
		return prefix + "/"
	}
	return prefix + path
}

func isMountedPreview(state PageState) bool {
	return strings.TrimSpace(state.MountPrefix) != ""
}

func previewPathForSessionID(state PageState, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	// ponytail: el proxy del preview vive en
	// /agent/sessions/{id}/preview/ (registrado en
	// internal/agent/http/preview_proxy.go). La V2 antes
	// usaba /agent/preview/{id}/ (ruta V2 con prefijo dedicado)
	// que era servida por un forwarder V2→V1; ese forwarder
	// ya no existe en el cutover. Si el cliente V2 genera la
	// URL vieja, el request cae al catch-all del home handler
	// y el usuario ve la home en lugar del preview. La fix:
	// apuntar a la ruta real del proxy.
	return appPath(state, "/agent/sessions/"+id+"/preview/")
}

func shellPreviewPath(state PageState) string {
	if isMountedPreview(state) {
		return ""
	}
	return previewPathForSessionID(state, activeSessionID(state))
}

func hostAgentSessionURL(state PageState) string {
	id := activeSessionID(state)
	if id == "" {
		return "/agent"
	}
	return "/agent?session=" + id
}
