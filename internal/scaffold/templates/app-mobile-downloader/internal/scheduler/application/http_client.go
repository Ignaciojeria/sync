package scheduler

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Ignaciojeria/ioc"
)

var _ = ioc.Register(NewInternalHTTPClient)

type InternalHTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewInternalHTTPClient() *InternalHTTPClient {
	return &InternalHTTPClient{
		baseURL: "http://localhost:8000",
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *InternalHTTPClient) Post(endpoint string) error {
	url := c.baseURL + endpoint
	resp, err := c.client.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}
