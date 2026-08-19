package honcho

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// Config agrupa los knobs del adapter. Lo construimos desde
// cmd/api leyendo os.Getenv(...) y se lo pasamos a NewAdapter.
type Config struct {
	BaseURL        string        // ej. "https://api.honcho.dev"
	APIKey         string        // bearer token
	WorkspaceID    string        // ej. "lastmile-agents" o un UUID
	RecallTimeout  time.Duration // timeout de Recall (default 2s)
	TokenBudget    int           // cap de tokens server-side para context (default 1000)
	SearchTopK     int           // top-k para search_query (default 8)
	MaxMessageChars int          // límite duro de Honcho por content (25000)
	MaxBatchSize   int           // límite duro de Honcho por batch (100)
}

// withDefaults devuelve una copia de c con defaults aplicados
// a los campos zero. No muta el input.
func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.honcho.dev"
	}
	if c.RecallTimeout == 0 {
		c.RecallTimeout = 2 * time.Second
	}
	if c.TokenBudget == 0 {
		c.TokenBudget = 1000
	}
	if c.SearchTopK == 0 {
		c.SearchTopK = 8
	}
	if c.MaxMessageChars == 0 {
		c.MaxMessageChars = 25000
	}
	if c.MaxBatchSize == 0 {
		c.MaxBatchSize = 100
	}
	return c
}

// Adapter implementa agentapp.MemoryProvider contra Honcho.
// Es seguro para uso concurrente desde el Manager.
type Adapter struct {
	cfg    Config
	client *Client
}

// NewAdapter construye el adapter. baseURL y apiKey son
// obligatorios (sin ellos no hay contra qué hablar); el resto
// toma defaults razonables.
func NewAdapter(cfg Config) (*Adapter, error) {
	cfg = cfg.withDefaults()
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("honcho: BaseURL is required")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, errors.New("honcho: APIKey is required")
	}
	if strings.TrimSpace(cfg.WorkspaceID) == "" {
		return nil, errors.New("honcho: WorkspaceID is required")
	}
	return &Adapter{
		cfg:    cfg,
		client: NewClient(cfg.BaseURL, cfg.APIKey, nil),
	}, nil
}

// Compile-time check: *Adapter implementa MemoryProvider.
var _ agentapp.MemoryProvider = (*Adapter)(nil)

// EnsurePeers garantiza que workspace, agent peer, user peer y
// session existen. Idempotente: Honcho devuelve 200 si ya
// existen. Si la session ya existía pero el user peer no
// estaba bindeado, lo agregamos con AddPeerToSession como
// red de seguridad.
func (a *Adapter) EnsurePeers(ctx context.Context, key agentapp.MemoryKey) error {
	ids, err := resolveIDs(key)
	if err != nil {
		return err
	}
	if err := a.client.CreateWorkspace(ctx, ids.WorkspaceID); err != nil {
		return fmt.Errorf("honcho: create workspace: %w", err)
	}
	if err := a.client.GetOrCreatePeer(ctx, ids.WorkspaceID, ids.AgentPeerID); err != nil {
		return fmt.Errorf("honcho: create agent peer: %w", err)
	}
	if err := a.client.GetOrCreatePeer(ctx, ids.WorkspaceID, ids.UserPeerID); err != nil {
		return fmt.Errorf("honcho: create user peer: %w", err)
	}
	peers := map[string]SessionPeerConfig{
		ids.AgentPeerID: {ObserveMe: false},
		ids.UserPeerID:  {ObserveMe: true},
	}
	if err := a.client.CreateSession(ctx, ids.WorkspaceID, ids.SessionID, peers); err != nil {
		// Si la session ya existía sin nuestros peers, agregamos
		// el user al menos (el agent probablemente ya estaba por
		// la creación anterior).
		if apiErr, ok := err.(*APIError); ok && httpConflictOrUnprocessable(apiErr.StatusCode) {
			if addErr := a.client.AddPeerToSession(ctx, ids.WorkspaceID, ids.SessionID, ids.UserPeerID, SessionPeerConfig{ObserveMe: true}); addErr != nil {
				return fmt.Errorf("honcho: session exists but add peer failed: %w", addErr)
			}
			return nil
		}
		return fmt.Errorf("honcho: create session: %w", err)
	}
	return nil
}

// Recall trae contexto relevante al query. El Text devuelto ya
// está formateado para inyectar como bloque de "memoria
// relevante". Si la respuesta viene vacía, devuelve
// MemoryContext{} (sin error) para que el caller decida si
// steer o no.
func (a *Adapter) Recall(ctx context.Context, key agentapp.MemoryKey, query string) (agentapp.MemoryContext, error) {
	ids, err := resolveIDs(key)
	if err != nil {
		return agentapp.MemoryContext{}, err
	}
	recallCtx, cancel := context.WithTimeout(ctx, a.cfg.RecallTimeout)
	defer cancel()

	sctx, err := a.client.GetSessionContext(recallCtx, ids.WorkspaceID, ids.SessionID, GetSessionContextOptions{
		Tokens:      a.cfg.TokenBudget,
		SearchQuery: query,
		SearchTopK:  a.cfg.SearchTopK,
		// peer_target es el peer del USER (no del agent). El
		// agente es nuevo en cada session (Fase 1 hasta que
		// llegue AgentID real), así que su representación está
		// vacía. El user es el peer con historial cross-session
		// — Honcho razona sobre sus mensajes y devuelve qué
		// sabemos del humano, qué temas discutió, qué
		// preferencias tiene. Eso es lo que queremos inyectar
		// como contexto antes del prompt.
		PeerTarget: ids.UserPeerID,
	})
	if err != nil {
		return agentapp.MemoryContext{}, fmt.Errorf("honcho: get session context: %w", err)
	}
	text := formatContext(sctx)
	if text == "" {
		return agentapp.MemoryContext{}, nil
	}
	return agentapp.MemoryContext{Text: text}, nil
}

// Remember persiste los mensajes en batches respetando los
// límites de Honcho (100 por batch, 25000 chars por content).
// Si un mensaje excede el límite, se trunca con sufijo
// explícito. Roles desconocidos se loggean y descartan.
func (a *Adapter) Remember(ctx context.Context, key agentapp.MemoryKey, msgs []agentapp.MemoryMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	ids, err := resolveIDs(key)
	if err != nil {
		return err
	}

	// Filtramos roles desconocidos y truncamos.
	prepared := make([]MessageCreate, 0, len(msgs))
	for _, m := range msgs {
		peerID := ids.peerIDForRole(m.Role)
		if peerID == "" {
			slog.Warn("honcho: discarding message with unknown role",
				"role", m.Role,
				"session_id", ids.SessionID,
			)
			continue
		}
		prepared = append(prepared, MessageCreate{
			Content:   truncateForHoncho(m.Text, a.cfg.MaxMessageChars),
			PeerID:    peerID,
			CreatedAt: m.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	if len(prepared) == 0 {
		return nil
	}

	// Chunking en batches de a.cfg.MaxBatchSize.
	for start := 0; start < len(prepared); start += a.cfg.MaxBatchSize {
		end := start + a.cfg.MaxBatchSize
		if end > len(prepared) {
			end = len(prepared)
		}
		batch := prepared[start:end]
		if err := a.client.CreateMessages(ctx, ids.WorkspaceID, ids.SessionID, batch); err != nil {
			return fmt.Errorf("honcho: create messages batch [%d:%d]: %w", start, end, err)
		}
	}
	return nil
}

// formatContext arma el string final que el Manager va a
// inyectar como steer. Devuelve "" si todos los campos están
// vacíos para que el caller no inyectue un bloque inútil.
//
// Formato:
//
//	<summary>...</summary>
//	<representation>...</representation>
//	<peer_card>
//	- item 1
//	- item 2
//	</peer_card>
func formatContext(sctx SessionContext) string {
	var b strings.Builder
	if sctx.Summary != nil && strings.TrimSpace(sctx.Summary.Content) != "" {
		b.WriteString("<summary>\n")
		b.WriteString(strings.TrimSpace(sctx.Summary.Content))
		b.WriteString("\n</summary>\n")
	}
	if sctx.PeerRepresentation != nil && strings.TrimSpace(*sctx.PeerRepresentation) != "" {
		b.WriteString("<representation>\n")
		b.WriteString(strings.TrimSpace(*sctx.PeerRepresentation))
		b.WriteString("\n</representation>\n")
	}
	if len(sctx.PeerCard) > 0 {
		b.WriteString("<peer_card>\n")
		for _, item := range sctx.PeerCard {
			if strings.TrimSpace(item) == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(item))
			b.WriteString("\n")
		}
		b.WriteString("</peer_card>\n")
	}
	return strings.TrimSpace(b.String())
}

// truncateForHoncho recorta el content a maxChars y agrega un
// sufijo explícito. Honcho responde 422 si content > 25000;
// preferimos truncar nosotros a perder el mensaje o fallar el
// flush completo.
func truncateForHoncho(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}
	const suffix = "\n…[truncated by honcho adapter]"
	if maxChars <= len(suffix) {
		return s[:maxChars]
	}
	return s[:maxChars-len(suffix)] + suffix
}

// httpConflictOrUnprocessable es un helper para clasificar
// errores de Honcho en EnsurePeers. Cuando la session ya existe
// con una config incompatible, Honcho puede devolver 409 o 422;
// en ese caso intentamos AddPeerToSession como fallback.
//
// Mantenido como función (no constante) porque el compilador
// no permite switch sobre constantes en inicialización de vars.
func httpConflictOrUnprocessable(status int) bool {
	return status == 409 || status == 422
}
