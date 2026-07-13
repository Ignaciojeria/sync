package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBalanceServiceFetchOK(t *testing.T) {
	t.Parallel()

	var sawAuth bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization") == "Bearer sag_test"
		if r.URL.Path != "/balance" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BalanceResponse{
			BalanceUSD: 1.234,
			ClientID:   "client-x",
		})
	}))
	defer ts.Close()

	svc := NewBalanceService(ts.URL, "sag_test")
	got, err := svc.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.BalanceUSD != 1.234 || got.ClientID != "client-x" {
		t.Errorf("Fetch() = %+v", got)
	}
	if !sawAuth {
		t.Error("server did not see Bearer token")
	}
}

func TestBalanceServiceFetchRequiresConfig(t *testing.T) {
	cases := []*BalanceService{
		NewBalanceService("", "key"),
		NewBalanceService("http://x", ""),
		nil,
	}
	for i, svc := range cases {
		if _, err := svc.Fetch(context.Background()); err == nil {
			t.Errorf("case %d: expected error for unconfigured service", i)
		}
	}
}

func TestBalanceServiceFetchNonOKStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("nope"))
	}))
	defer ts.Close()

	svc := NewBalanceService(ts.URL, "key")
	if _, err := svc.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestBalanceServiceFetchInvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	defer ts.Close()

	svc := NewBalanceService(ts.URL, "key")
	if _, err := svc.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for malformed body")
	}
}
