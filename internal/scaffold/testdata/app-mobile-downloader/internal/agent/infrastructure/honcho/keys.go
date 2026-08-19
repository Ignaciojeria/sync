package honcho

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	agentapp "lastmile-agents/internal/agent/application"
)

// honchoIDPattern es la regex que Honcho valida en todos los IDs
// (workspace, peer, session). Lo respetamos acá para evitar
// 422 silenciosos en el adapter.
//
// Lesson learned: la doc oficial de Honcho muestra peer IDs con
// prefijos tipo "user:alice@example.com", pero el API rechaza el
// `:` y `@` con 422. El adapter sanitiza antes de mandar.
const honchoIDPattern = "^[a-zA-Z0-9_-]+$"

// agentPeerPrefix es el prefijo del peer ID del agente en Honcho.
// En Fase 1 del plan developer-teams esto va a ser
// "agent-" + key.AgentID; mientras tanto usamos SessionID como
// proxy (un peer por session de agente, no un peer por agente
// persistente). Documentar en el README que esta decisión pierde
// memoria cross-session hasta que llegue Fase 1.
const agentPeerPrefix = "agent-"

// userPeerPrefix es el prefijo del peer ID humano. Como Honcho
// rechaza caracteres especiales en IDs (el email tiene @ y .),
// usamos SHA256 del email normalizado (lowercase + trim) y
// tomamos los primeros 16 hex chars. Esto da:
//   - estabilidad: mismo email → mismo peer ID siempre
//   - privacidad: el email no aparece en el peer ID que queda
//     almacenado en Honcho
//   - colisión despreciable: 64 bits de entropía es suficiente
//     para nuestro orden de magnitud (cientos de usuarios)
const userPeerPrefix = "user-"

// resolvedIDs agrupa los IDs que el adapter pasa al client.
// Vive acá (no en application/) porque es específico del
// keying de Honcho; otros backends de memoria podrían resolver
// la misma MemoryKey a otra cosa.
type resolvedIDs struct {
	WorkspaceID string
	SessionID   string
	AgentPeerID string
	UserPeerID  string
}

// resolveIDs convierte una MemoryKey en los IDs concretos que
// el client espera. Devuelve error si falta SessionID o
// UserEmail — son los dos campos mínimos para que un
// EnsurePeers sea útil.
func resolveIDs(key agentapp.MemoryKey) (resolvedIDs, error) {
	wsID := strings.TrimSpace(key.WorkspaceID)
	sessID := strings.TrimSpace(key.SessionID)
	userEmail := strings.TrimSpace(key.UserEmail)
	if sessID == "" {
		return resolvedIDs{}, fmt.Errorf("honcho: MemoryKey.SessionID is required")
	}
	if userEmail == "" {
		return resolvedIDs{}, fmt.Errorf("honcho: MemoryKey.UserEmail is required")
	}
	// Validar que los IDs que vamos a mandar cumplan el patrón
	// de Honcho. Si SessionID tiene chars raros, fallamos
	// explícito (no tiene sentido sanitizar y seguir).
	if !matchesHonchoPattern(sessID) {
		return resolvedIDs{}, fmt.Errorf("honcho: MemoryKey.SessionID %q contains characters Honcho does not accept", sessID)
	}
	agentPeerID := agentPeerPrefix + sessID
	if key.AgentID != "" {
		// Fase 1 del plan developer-teams: cuando hay AgentID
		// estable, el peer del agente es uno por agente real,
		// no por session. Esto habilita memoria cross-session.
		// El AgentID debe venir ya sanitizado (ulid, uuid, etc.)
		// — validamos acá por seguridad.
		if !matchesHonchoPattern(key.AgentID) {
			return resolvedIDs{}, fmt.Errorf("honcho: MemoryKey.AgentID %q contains characters Honcho does not accept", key.AgentID)
		}
		agentPeerID = agentPeerPrefix + key.AgentID
	}
	return resolvedIDs{
		WorkspaceID: wsID,
		SessionID:   sessID,
		AgentPeerID: agentPeerID,
		UserPeerID:  userPeerIDFor(userEmail),
	}, nil
}

// peerIDForRole mapea el campo Role de MemoryMessage al peer
// ID concreto. El adapter es quien decide esto, no el Manager:
// el Manager sólo dice "user" o "assistant", y el adapter sabe
// qué peer ID le corresponde en Honcho.
func (r resolvedIDs) peerIDForRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return r.UserPeerID
	case "assistant":
		return r.AgentPeerID
	default:
		// Rol desconocido: no mandamos. El adapter loggea y
		// descarta (Remember devuelve error si detecta esto).
		return ""
	}
}

// userPeerIDFor genera el peer ID del usuario a partir de su
// email. Usa SHA256 del email normalizado (lowercase + trim) y
// toma los primeros 16 hex chars. Ver userPeerPrefix para
// rationale.
func userPeerIDFor(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	sum := sha256.Sum256([]byte(normalized))
	return userPeerPrefix + hex.EncodeToString(sum[:8])
}

// matchesHonchoPattern chequea que un string sea aceptable
// como ID por el API de Honcho. La regex es
// ^[a-zA-Z0-9_-]+$; ningún otro carácter es válido.
func matchesHonchoPattern(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
