package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
	RawBody    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("api error (%d %s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("api error (%d): %s", e.StatusCode, e.Message)
}

func NewClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type createProjectRequest struct {
	Name       string `json:"name"`
	Public     bool   `json:"public"`
	Visibility string `json:"visibility,omitempty"`
}

type CreateProjectResponse struct {
	ProjectID          string `json:"projectId"`
	Name               string `json:"name"`
	Slug               string `json:"slug"`
	Path               string `json:"path"`
	Subdomain          string `json:"subdomain"`
	Status             string `json:"status"`
	MutagenDestination string `json:"mutagenDestination,omitempty"`
	MutagenSessionName string `json:"mutagenSessionName,omitempty"`
	VMName             string `json:"vmName,omitempty"`
	VMHTTPSURL         string `json:"vmHttpsUrl,omitempty"`
	VMSshDest          string `json:"vmSshDest,omitempty"`
	VMSshPrivateKey    string `json:"vmSshPrivateKey,omitempty"`
	ProjectAPIToken    string `json:"projectApiToken,omitempty"`
}

func (c *Client) CreateProject(ctx context.Context, name string) (*CreateProjectResponse, error) {
	if c.token == "" {
		return nil, fmt.Errorf("falta token (usa EINAR_TOKEN o 'login --token')")
	}
	payload := createProjectRequest{Name: name, Public: true, Visibility: "public"}
	var out CreateProjectResponse
	if err := c.doWithRetry(ctx, http.MethodPost, "/api/projects", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) doWithRetry(ctx context.Context, method, path string, in any, out any) error {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		err := c.do(ctx, method, path, in, out)
		if err == nil {
			return nil
		}

		apiErr := &APIError{}
		if ok := AsAPIError(err, apiErr); ok {
			if apiErr.StatusCode < 500 || apiErr.StatusCode > 599 || attempt == maxAttempts {
				return err
			}
		} else {
			if !isTransientNetworkError(err) || attempt == maxAttempts {
				return err
			}
		}

		backoff := time.Duration(math.Pow(2, float64(attempt-1))*200) * time.Millisecond
		jitter := time.Duration(rand.Intn(100)) * time.Millisecond
		time.Sleep(backoff + jitter)
	}
	return fmt.Errorf("no se pudo completar request")
}

func (c *Client) do(ctx context.Context, method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp.StatusCode, respBody)
	}

	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return err
	}
	return nil
}

func parseAPIError(status int, body []byte) error {
	type wrappedErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	apiErr := &APIError{StatusCode: status, RawBody: string(body), Message: string(body)}
	var parsed wrappedErr
	if err := json.Unmarshal(body, &parsed); err == nil {
		if parsed.Error.Code != "" || parsed.Error.Message != "" {
			apiErr.Code = parsed.Error.Code
			apiErr.Message = parsed.Error.Message
			return apiErr
		}
		if parsed.Code != "" || parsed.Message != "" {
			apiErr.Code = parsed.Code
			apiErr.Message = parsed.Message
			return apiErr
		}
	}
	return apiErr
}

func AsAPIError(err error, target *APIError) bool {
	e, ok := err.(*APIError)
	if !ok {
		return false
	}
	*target = *e
	return true
}

func isTransientNetworkError(err error) bool {
	if nerr, ok := err.(net.Error); ok {
		return nerr.Timeout() || nerr.Temporary()
	}
	return false
}
