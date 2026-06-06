package machineauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	GrantType     string
	TokenEndpoint string
	ClientID      string
	ClientSecret  string
	Audience      string
	Scopes        []string
}

type TokenSource struct {
	httpClient *http.Client
	cfg        Config

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func NewTokenSource(cfg Config) (*TokenSource, error) {
	cfg.GrantType = strings.TrimSpace(cfg.GrantType)
	if cfg.GrantType == "" {
		cfg.GrantType = "client_credentials"
	}
	cfg.TokenEndpoint = strings.TrimSpace(cfg.TokenEndpoint)
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.Audience = strings.TrimSpace(cfg.Audience)
	if cfg.GrantType != "client_credentials" {
		return nil, fmt.Errorf("machine auth grantType no soportado: %s", cfg.GrantType)
	}
	if cfg.TokenEndpoint == "" || cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("machine auth incompleto: tokenEndpoint, clientId y clientSecret son requeridos")
	}
	return &TokenSource{
		httpClient: &http.Client{Timeout: 20 * time.Second},
		cfg:        cfg,
	}, nil
}

func (s *TokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if strings.TrimSpace(s.token) != "" && time.Until(s.expiresAt) > 30*time.Second {
		return s.token, nil
	}

	form := url.Values{}
	form.Set("grant_type", s.cfg.GrantType)
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	if s.cfg.Audience != "" {
		form.Set("audience", s.cfg.Audience)
	}
	if scopes := strings.Join(nonEmpty(s.cfg.Scopes), " "); scopes != "" {
		form.Set("scope", scopes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.TokenEndpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode machine token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(out.Description)
		if msg == "" {
			msg = strings.TrimSpace(out.Error)
		}
		if msg == "" {
			msg = resp.Status
		}
		return "", fmt.Errorf("machine token request failed: %s", msg)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", fmt.Errorf("machine token response sin access_token")
	}

	ttl := time.Duration(out.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	s.token = strings.TrimSpace(out.AccessToken)
	s.expiresAt = time.Now().Add(ttl)
	return s.token, nil
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
