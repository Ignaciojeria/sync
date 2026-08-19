// Package honcho implementa el adapter del bounded context agent
// contra el servicio externo Honcho (https://honcho.dev). Vive en
// infrastructure/ por convención del proyecto: la interfaz
// MemoryProvider está en application/, la implementación HTTP
// vive acá.
//
// No usamos el SDK oficial de Honcho porque no hay versión Go;
// hablamos HTTP directo contra api.honcho.dev.
package honcho

// Tipos request/response del API v3 de Honcho. Documentación
// oficial: https://honcho.dev/docs/v3/api-reference. Mantener
// estos structs en sync con el OpenAPI; no agregar campos que
// Honcho no envíe/reciba para que un cambio de schema se note
// rápido.

// WorkspaceCreate es el body de POST /v3/workspaces.
// El backend hace upsert: si el id ya existe, lo devuelve; si no, lo crea.
type WorkspaceCreate struct {
	ID            string                 `json:"id"`
	Metadata      map[string]any         `json:"metadata,omitempty"`
	Configuration map[string]any         `json:"configuration,omitempty"`
}

// PeerCreate es el body de POST /v3/workspaces/{wid}/peers.
// Idempotente: si el peer ya existe, lo devuelve.
type PeerCreate struct {
	ID            string         `json:"id"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Configuration map[string]any `json:"configuration,omitempty"`
}

// SessionPeerConfig controla qué razona Honcho sobre este peer
// dentro de la session. observe_me=false para el agente (su
// razonamiento es determinista); observe_me=true para el user.
type SessionPeerConfig struct {
	ObserveMe    bool `json:"observe_me,omitempty"`
	ObserveOthers bool `json:"observe_others,omitempty"`
}

// SessionCreate es el body de POST /v3/workspaces/{wid}/sessions.
// El mapa Peers key=peer_id, value=config. Honcho crea los peers
// que no existan y los bindea a la session en una sola llamada.
type SessionCreate struct {
	ID            string                      `json:"id"`
	Metadata      map[string]any              `json:"metadata,omitempty"`
	Peers         map[string]SessionPeerConfig `json:"peers,omitempty"`
	Configuration map[string]any              `json:"configuration,omitempty"`
}

// AddPeerToSessionRequest es el body de
// POST /v3/workspaces/{wid}/sessions/{sid}/peers. A diferencia
// de SessionCreate, esta ruta recibe un ARRAY de peers, no un map.
type AddPeerToSessionRequest struct {
	PeerID        string             `json:"peer_id"`
	ObserveMe     bool               `json:"observe_me,omitempty"`
	ObserveOthers bool               `json:"observe_others,omitempty"`
}

// MessageCreate es un item dentro de MessageBatchCreate.
// El límite de Content es 25000 chars; el caller debe truncar
// antes de enviar.
type MessageCreate struct {
	Content   string         `json:"content"`
	PeerID    string         `json:"peer_id"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"` // ISO8601
}

// MessageBatchCreate es el body de
// POST /v3/workspaces/{wid}/sessions/{sid}/messages.
// Honcho limita a 100 mensajes por batch.
type MessageBatchCreate struct {
	Messages []MessageCreate `json:"messages"`
}

// GetSessionContextOptions son los query params de
// GET /v3/workspaces/{wid}/sessions/{sid}/context. El más
// importante para nuestro caso de uso es Tokens (server-side
// cap) y PeerTarget (incluye la representación del peer en la
// respuesta).
type GetSessionContextOptions struct {
	Tokens          int    // 0 = exhaustivo (cap interno 100k)
	SearchQuery     string // filtra conclusiones por relevancia semántica
	SearchTopK      int    // 1-100, default si no se setea
	SearchMaxDistance float64 // 0.0-1.0
	Summary         bool   // incluir summary (default true)
	PeerTarget      string // peer_id cuya representación queremos incluir
}

// SessionContext es la respuesta de GET .../context. Sólo
// definimos los campos que vamos a usar; Honcho puede devolver
// más y los ignoramos (json.Unmarshal no falla por campos extra
// si usamos una struct sin json:"-" en el decoder).
type SessionContext struct {
	ID                string   `json:"id"`
	Messages          []Message `json:"messages"`
	Summary           *Summary  `json:"summary,omitempty"`
	PeerRepresentation *string  `json:"peer_representation,omitempty"`
	PeerCard          []string  `json:"peer_card,omitempty"`
}

// Message es la representación de un mensaje en la respuesta
// de context. Sólo leemos Content y TokenCount.
type Message struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	PeerID     string `json:"peer_id"`
	SessionID  string `json:"session_id"`
	TokenCount int    `json:"token_count"`
}

// Summary es el resumen agregado de la session.
type Summary struct {
	Content     string `json:"content"`
	MessageID   string `json:"message_id"`
	SummaryType string `json:"summary_type"`
	TokenCount  int    `json:"token_count"`
}
