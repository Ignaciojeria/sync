package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SessionCostResponse struct {
	SessionID        string  `json:"session_id"`
	RequestCount     int     `json:"request_count"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Currency         string  `json:"currency,omitempty"`
	// ponytail: model_alias + context_window son metadata del
	// modelo activo en el gateway. model_alias identifica el
	// modelo que está respondiendo (ej. "minimax/m3") y
	// context_window es el límite máximo de tokens del
	// contexto (ej. 1000000 para M3). Los devolvemos en el
	// response para que el V2 chat pueda renderizar un badge
	// con el modelo + un indicador de uso del contexto.
	ModelAlias    string `json:"model_alias,omitempty"`
	ContextWindow int64  `json:"context_window,omitempty"`
	// ponytail: current_prompt_tokens es el size del context
	// de la ÚLTIMA llamada a la API (no acumulado). Lo usa la
	// UI V2 para mostrar el % de uso del context window en
	// tiempo real, en lugar del acumulado total de la sesión
	// (que crecía con cada request y daba badges del 60%+
	// incluso cuando el context real era <5%). El gateway lo
	// calcula a partir del último request; si el gateway no lo
	// devuelve (versiones viejas), caemos al promedio de la
	// sesión como fallback razonable.
	CurrentPromptTokens int64 `json:"current_prompt_tokens,omitempty"`
}

type SessionCostService struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewSessionCostService(baseURL, apiKey string) *SessionCostService {
	return &SessionCostService{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *SessionCostService) Fetch(ctx context.Context, sessionID string) (SessionCostResponse, error) {
	if s == nil || s.apiKey == "" || s.baseURL == "" {
		return SessionCostResponse{}, fmt.Errorf("gateway: not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return SessionCostResponse{}, fmt.Errorf("gateway: session_id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/session-cost?session_id="+url.QueryEscape(sessionID), nil)
	if err != nil {
		return SessionCostResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return SessionCostResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SessionCostResponse{}, fmt.Errorf("gateway: status %d", resp.StatusCode)
	}

	var out SessionCostResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SessionCostResponse{}, err
	}
	if strings.TrimSpace(out.Currency) == "" {
		out.Currency = "USD"
	}
	// ponytail: defaults para el badge del modelo + context window.
	// Si el gateway real ya devuelve model_alias/context_window,
	// no pisamos esos valores. Solo los rellenamos si vienen
	// vacíos (caso del gateway mock que todavía no conoce estos
	// campos). El objetivo es que la UI V2 muestre el badge desde
	// el primer render server-side, sin esperar a un deploy del
	// gateway real.
	if strings.TrimSpace(out.ModelAlias) == "" {
		out.ModelAlias = "minimax/m3"
	}
	if out.ContextWindow <= 0 {
		out.ContextWindow = 1000000
	}
	// ponytail: fallback defensivo. Si el gateway (en
	// versiones viejas) no devuelve current_prompt_tokens,
	// estimamos con el promedio. Una vez que el gateway
	// devuelva el campo, usamos el real.
	if out.CurrentPromptTokens == 0 && out.RequestCount > 0 {
		out.CurrentPromptTokens = out.PromptTokens / int64(out.RequestCount)
	}
	return out, nil
}
