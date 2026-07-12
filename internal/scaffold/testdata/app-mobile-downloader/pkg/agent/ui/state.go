package ui

import (
	"fmt"
	"strings"
	"time"

	agentapp "testboi1/pkg/agent/application"
)

type PageState struct {
	Sessions      []agentapp.Session
	ActiveSession *agentapp.Session
	MountPrefix   string
	// ActiveSessionID permite renderizar la shell del chat sin depender
	// del runtime local del agente en el web-server. La UI se hidrata
	// después pegándole al worker vía BFF.
	ActiveSessionID string
	DefaultCWD      string
	DefaultModel    string
	// CurrentView controla la navegación inferior móvil del agente.
	// Valores esperados: "dashboard" (default) o "providers".
	CurrentView string
	// UserJWT es el token crudo (IDToken de Casdoor) del usuario
	// autenticado. Se embebe en el HTML como data-user-jwt y el JS del
	// cliente lo usa en `Authorization: Bearer ...` para hablar con el
	// agent-worker (que valida JWT contra Casdoor, no contra cookie).
	// Si está vacío, la UI no puede enviar prompts al worker.
	UserJWT string
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

func statusBadgeClass(status agentapp.SessionStatus) string {
	switch status {
	case agentapp.SessionStatusRunning:
		return "badge badge-warning badge-soft text-[0.68rem]"
	case agentapp.SessionStatusError:
		return "badge badge-error badge-soft text-[0.68rem]"
	default:
		return "badge badge-ghost text-[0.68rem]"
	}
}

func statusLabel(status agentapp.SessionStatus) string {
	if strings.TrimSpace(string(status)) == "" {
		return "idle"
	}
	return string(status)
}

func sessionMetaValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isActiveSession(state PageState, session agentapp.Session) bool {
	return state.ActiveSession != nil && state.ActiveSession.ID == session.ID
}

func sessionCountLabel(sessions []agentapp.Session) string {
	return fmt.Sprintf("%d", len(sessions))
}

func sessionLinkClass(state PageState, session agentapp.Session) string {
	className := "group block rounded-box border border-transparent px-2.5 py-2.5 transition-all duration-200 hover:-translate-y-px hover:border-base-300/60 hover:bg-base-100/60"
	if isActiveSession(state, session) {
		return className + " border-base-300/70 bg-base-100/72"
	}
	return className
}

func shellSessionDate(t time.Time) string {
	if t.IsZero() {
		return "sin fecha"
	}
	return t.Format("1/2/2006")
}

func shellSessionCWD(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "cwd pendiente"
	}
	return value
}

func shellTitle(state PageState) string {
	if state.ActiveSession == nil || strings.TrimSpace(state.ActiveSession.Title) == "" {
		return "sync4.run"
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

func previewPathForSessionID(state PageState, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return appPath(state, "/agent/sessions/"+id+"/preview/")
}

func shellPreviewPath(state PageState) string {
	if isMountedPreview(state) {
		return ""
	}
	return previewPathForSessionID(state, activeSessionID(state))
}

func isMountedPreview(state PageState) bool {
	return strings.TrimSpace(state.MountPrefix) != ""
}

func currentView(state PageState) string {
	if strings.TrimSpace(state.CurrentView) == "providers" {
		return "providers"
	}
	return "dashboard"
}

func isDashboardView(state PageState) bool {
	return currentView(state) == "dashboard"
}

func isProvidersView(state PageState) bool {
	return currentView(state) == "providers"
}
