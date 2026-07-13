package ui

import (
	"strings"

	agentapp "fixtests1/internal/agent/application"
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

func hostAgentSessionURL(state PageState) string {
	id := activeSessionID(state)
	if id == "" {
		return "/agent"
	}
	return "/agent?session=" + id
}
