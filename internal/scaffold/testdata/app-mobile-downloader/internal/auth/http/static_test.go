package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func TestServeStaticFile(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"logo.jpeg", "logo.svg", "login.jpeg", "login-bg.svg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[binary-data:"+name+"]"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd(): %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(): %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	fs := fuego.NewServer()
	s := &server.Server{Server: fs}
	registerStaticAssets(s)
	ts := httptest.NewServer(fs.Mux)
	t.Cleanup(ts.Close)

	for _, path := range []string{"/logo.jpeg", "/logo.svg", "/login.jpeg", "/login-bg.svg"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("Get(%s): %v", path, err)
		}
		t.Cleanup(func() { _ = res.Body.Close() })

		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 for %s", res.StatusCode, path)
		}
	}
}
