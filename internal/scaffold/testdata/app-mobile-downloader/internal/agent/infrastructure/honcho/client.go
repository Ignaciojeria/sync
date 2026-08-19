package honcho

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultHTTPTimeout es el timeout por-request cuando el caller
// no setea uno en el context. Honcho suele responder en <500ms
// para endpoints de lectura; 10s da margen para razonamiento
// inicial sobre un peer nuevo.
const DefaultHTTPTimeout = 10 * time.Second

// Client es un wrapper HTTP fino contra el API v3 de Honcho.
// No tiene estado mutable más allá del httpClient; es seguro
// para uso concurrente desde múltiples goroutines del adapter.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient construye un Client. baseURL debe ser el origen
// (ej. "https://api.honcho.dev") sin path. apiKey es el bearer
// token de https://app.honcho.dev/api-keys.
//
// Si httpClient es nil, se usa uno con timeout DefaultHTTPTimeout.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: DefaultHTTPTimeout}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

// CreateWorkspace hace upsert del workspace. Honcho devuelve 200
// si el workspace ya existía, 201 si lo creó; ambos son éxito.
func (c *Client) CreateWorkspace(ctx context.Context, id string) error {
	body := WorkspaceCreate{ID: id}
	// 201/200 ambos indican éxito; no validamos status code
	// específico porque Honcho puede devolver cualquiera.
	return c.do(ctx, http.MethodPost, "/v3/workspaces", body, nil)
}

// GetOrCreatePeer hace upsert del peer dentro del workspace.
func (c *Client) GetOrCreatePeer(ctx context.Context, workspaceID, peerID string) error {
	body := PeerCreate{ID: peerID}
	path := fmt.Sprintf("/v3/workspaces/%s/peers", url.PathEscape(workspaceID))
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// CreateSession crea la session y bindea los peers indicados en
// una sola llamada. Si la session ya existe, Honcho la devuelve
// tal cual; el binding de peers es idempotente.
func (c *Client) CreateSession(ctx context.Context, workspaceID, sessionID string, peers map[string]SessionPeerConfig) error {
	body := SessionCreate{ID: sessionID, Peers: peers}
	path := fmt.Sprintf("/v3/workspaces/%s/sessions", url.PathEscape(workspaceID))
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// AddPeerToSession agrega un peer a una session existente.
// Útil cuando llega el user email mid-session.
func (c *Client) AddPeerToSession(ctx context.Context, workspaceID, sessionID, peerID string, cfg SessionPeerConfig) error {
	body := AddPeerToSessionRequest{
		PeerID:        peerID,
		ObserveMe:     cfg.ObserveMe,
		ObserveOthers: cfg.ObserveOthers,
	}
	path := fmt.Sprintf("/v3/workspaces/%s/sessions/%s/peers",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// GetSessionContext devuelve el contexto de la session con cap
// de tokens server-side. Si opts.Tokens == 0, Honcho usa su
// default interno (cap 100k, sin summary si no hay).
func (c *Client) GetSessionContext(ctx context.Context, workspaceID, sessionID string, opts GetSessionContextOptions) (SessionContext, error) {
	q := url.Values{}
	if opts.Tokens > 0 {
		q.Set("tokens", fmt.Sprintf("%d", opts.Tokens))
	}
	if opts.SearchQuery != "" {
		q.Set("search_query", opts.SearchQuery)
	}
	if opts.SearchTopK > 0 {
		q.Set("search_top_k", fmt.Sprintf("%d", opts.SearchTopK))
	}
	if opts.SearchMaxDistance > 0 {
		q.Set("search_max_distance", fmt.Sprintf("%g", opts.SearchMaxDistance))
	}
	// Summary se manda explícito; Honcho default es true pero queremos
	// ser explícitos en el contrato del adapter.
	q.Set("summary", "true")
	if opts.PeerTarget != "" {
		q.Set("peer_target", opts.PeerTarget)
	}
	path := fmt.Sprintf("/v3/workspaces/%s/sessions/%s/context",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var out SessionContext
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return SessionContext{}, err
	}
	return out, nil
}

// CreateMessages persiste un batch de mensajes en la session.
// Honcho limita a 100 por request; el adapter es responsable de
// chunkear antes de llamar.
func (c *Client) CreateMessages(ctx context.Context, workspaceID, sessionID string, msgs []MessageCreate) error {
	if len(msgs) == 0 {
		return nil // no-op, evitamos un round-trip
	}
	if len(msgs) > 100 {
		return fmt.Errorf("honcho: CreateMessages batch size %d exceeds limit 100", len(msgs))
	}
	body := MessageBatchCreate{Messages: msgs}
	path := fmt.Sprintf("/v3/workspaces/%s/sessions/%s/messages",
		url.PathEscape(workspaceID), url.PathEscape(sessionID))
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// do es el helper único de HTTP. Si out != nil, decodifica JSON
// en él. Cualquier status >= 400 devuelve error con cuerpo
// incluido para debugging.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("honcho: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("honcho: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("honcho: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("honcho: read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Method:     method,
			Path:       path,
			Body:       truncateForLog(string(respBody), 512),
		}
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("honcho: decode response: %w (body: %s)", err, truncateForLog(string(respBody), 512))
		}
	}
	return nil
}

// APIError es el error que devuelve el client cuando Honcho
// responde con status >= 400. Incluye status, método, path y
// un preview del body truncado para logs.
type APIError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("honcho: %s %s: %d %s", e.Method, e.Path, e.StatusCode, e.Body)
}

// truncateForLog limita el body en errores para no inflar logs.
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
