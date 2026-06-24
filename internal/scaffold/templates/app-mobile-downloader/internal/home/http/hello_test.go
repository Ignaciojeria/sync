package home

import (
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

	bodyBuf := make([]byte, 4096)
	n, _ := res.Body.Read(bodyBuf)
	if !strings.Contains(string(bodyBuf[:n]), "Hello world") {
		t.Fatalf("expected body to contain 'Hello world', got %q", string(bodyBuf[:n]))
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
