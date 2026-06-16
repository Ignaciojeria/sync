package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"app-mobile-downloader/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func TestHelloWorldHandler(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	helloWorldHandler(s)

	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
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
	if !strings.Contains(string(body), "Hello world") || !strings.Contains(string(body), "Login successful.") {
		t.Fatalf("unexpected body: %q", string(body))
	}
}
