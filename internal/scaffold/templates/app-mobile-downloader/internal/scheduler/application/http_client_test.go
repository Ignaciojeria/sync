package scheduler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInternalHTTPClientPost(t *testing.T) {
	t.Run("success returns nil on 2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "{}" {
				t.Errorf("body = %q", string(body))
			}
			w.WriteHeader(http.StatusAccepted)
		}))
		defer srv.Close()

		c := &InternalHTTPClient{
			baseURL: srv.URL,
			client:  &http.Client{},
		}

		// Override endpoint to /trigger; relative to baseURL.
		if err := c.Post("/trigger"); err != nil {
			t.Fatalf("Post() error = %v", err)
		}
	})

	t.Run("returns error on non-2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		c := &InternalHTTPClient{baseURL: srv.URL, client: &http.Client{}}
		err := c.Post("/x")
		if err == nil {
			t.Fatal("expected error on non-2xx")
		}
	})

	t.Run("returns error when underlying transport fails", func(t *testing.T) {
		// httptest server is taken down before invoking Post() to simulate a network error.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close()

		c := &InternalHTTPClient{baseURL: url, client: &http.Client{}}
		if err := c.Post("/x"); err == nil {
			t.Fatal("expected transport error")
		}
	})
}

func TestNewInternalHTTPClient(t *testing.T) {
	c := NewInternalHTTPClient()
	if c.baseURL != "http://localhost:8000" {
		t.Fatalf("baseURL = %q", c.baseURL)
	}
	if c.client == nil || c.client.Timeout == 0 {
		t.Fatalf("expected non-nil http client with timeout")
	}
}
