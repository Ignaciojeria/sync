package editor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func TestEditorViewHandler(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	editorViewHandler(s)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/editor-view")
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

	bodyBuf := make([]byte, 8192)
	n, _ := res.Body.Read(bodyBuf)
	body := string(bodyBuf[:n])
	if !strings.Contains(body, "/editor") {
		t.Fatalf("expected iframe src to /editor, body=%q", body)
	}
}

func TestEditorRegister(t *testing.T) {
	fs := fuego.NewServer()
	s := &server.Server{Server: fs}

	// Replace upstream URL with an httptest server so editorHandler can register routes without crashing on the default address.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	t.Setenv("EDITOR_UPSTREAM_URL", upstream.URL)

	// Confirm Register wires routes by smoke-hitting an editor path.
	Register(s)
	ts := httptest.NewServer(fs.Mux)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/ping")
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (proxied no-content)", res.StatusCode)
	}

	// And editor-view should be reachable too.
	res2, err := http.Get(ts.URL + "/editor-view")
	if err != nil {
		t.Fatalf("Get() editor-view: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("editor-view status = %d, want 200", res2.StatusCode)
	}
}
