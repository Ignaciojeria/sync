package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Shape real confirmado del gateway: { "client_id": "...", "balance_usd": 0.353471 }.
// El sufijo "_usd" en el nombre del campo confirma que la moneda es siempre USD.
type BalanceResponse struct {
	BalanceUSD float64 `json:"balance_usd"`
	ClientID   string  `json:"client_id"`
}

type BalanceService struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewBalanceService(baseURL, apiKey string) *BalanceService {
	return &BalanceService{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *BalanceService) Fetch(ctx context.Context) (BalanceResponse, error) {
	if s == nil || s.apiKey == "" || s.baseURL == "" {
		return BalanceResponse{}, fmt.Errorf("gateway: not configured")
	}
	// ponytail: el path /balance vive en el mismo base URL que las
	// completions. Mismo gateway, distinto endpoint.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/balance", nil)
	if err != nil {
		return BalanceResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return BalanceResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return BalanceResponse{}, fmt.Errorf("gateway: status %d", resp.StatusCode)
	}

	var out BalanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return BalanceResponse{}, err
	}
	return out, nil
}
