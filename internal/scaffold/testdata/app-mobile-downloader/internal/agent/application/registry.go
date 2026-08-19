package application

import "strings"

// AgentDescriptor describe un agente disponible para crear sesiones.
// Cada agente vive bajo agents/<ID>/ en el repo, con su propio .pi/ y
// AGENTS.md que el runner siembra dentro del sandbox al iniciar la
// sesión.
//
// Por ahora el único agente es "develop"; cuando aparezcan más
// (reviewer, docs, etc.) se suman a DefaultAgents. El Manager
// resuelve AgentID vacío a DefaultAgentID() cuando el caller no
// especifica uno.
type AgentDescriptor struct {
	// ID es el identificador estable del agente (slug). Se usa como
	// nombre de carpeta bajo agents/, como valor persistido en
	// Session.AgentID y como parámetro de la API.
	ID string
	// Label es el nombre humano que se muestra en la UI (ej. sidenav).
	Label string
	// Default marca cuál es el agente por defecto cuando el caller
	// no pasa AgentID. Sólo un descriptor puede tenerlo en true; si
	// hubiera más de uno, el primero gana (defensivo).
	Default bool
}

// DefaultAgents es el registry hardcoded de agentes disponibles.
// Esta lista es deliberadamente chica: extenderla es decisión de
// producto (cada agente nuevo implica mantener un workspace
// separado en agents/<id>/). Si en el futuro se quiere cargar
// desde config o descubrimiento dinámico, este slice es el único
// punto a tocar.
var DefaultAgents = []AgentDescriptor{
	{ID: "develop", Label: "Develop", Default: true},
}

// DefaultAgentID devuelve el ID del agente por defecto. Si por
// error de configuración no hubiera ninguno marcado Default, cae
// al primero de la lista y, en último caso, a "develop" como
// red de seguridad (el agente que siempre existe en el repo).
func DefaultAgentID() string {
	for _, a := range DefaultAgents {
		if a.Default {
			return a.ID
		}
	}
	if len(DefaultAgents) > 0 {
		return DefaultAgents[0].ID
	}
	return "develop"
}

// ResolveAgentID normaliza el AgentID que llega del caller. Devuelve
// el ID resuelto y un bool que indica si el valor era válido
// (estaba en el registry). Si vacío o desconocido, cae al
// DefaultAgentID() — esto preserva la API actual donde el caller
// podía no pasar AgentID.
func ResolveAgentID(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return DefaultAgentID(), false
	}
	for _, a := range DefaultAgents {
		if a.ID == trimmed {
			return a.ID, true
		}
	}
	return DefaultAgentID(), false
}
