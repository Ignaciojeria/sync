package home

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func TestHomeHandler(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	homeHandler(s)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("content-type = %q", got)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	bodyText := string(body)
	checks := []string{
		"Tu workspace persistente para operar agentes.",
		"Abrir consola",
		"Ver design system",
	}
	for _, want := range checks {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("expected body to contain %q, got %q", want, bodyText)
		}
	}
}

func TestHomeRegister(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	Register(s)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}
