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
	return out, nil
}
