package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCostServiceFetch(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sag_test" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.URL.Path; got != "/session-cost" {
			t.Fatalf("path = %q", got)
		}
		if got := r.URL.Query().Get("session_id"); got != "sess_123" {
			t.Fatalf("session_id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"sess_123","request_count":14,"estimated_cost_usd":0.1832,"prompt_tokens":120340,"completion_tokens":18221,"total_tokens":138561}`))
	}))
	defer ts.Close()

	svc := NewSessionCostService(ts.URL, "sag_test")
	got, err := svc.Fetch(context.Background(), "sess_123")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.SessionID != "sess_123" || got.RequestCount != 14 || got.Currency != "USD" {
		t.Fatalf("Fetch() = %+v", got)
	}
	if got.TotalTokens != 138561 {
		t.Fatalf("TotalTokens = %d", got.TotalTokens)
	}
}
