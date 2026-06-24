package scaffold

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterializeAppMobileDownloader(t *testing.T) {
	dir := t.TempDir()
	if err := MaterializeAppMobileDownloader(dir, "my-project", " Owner@Example.COM "); err != nil {
		t.Fatalf("MaterializeAppMobileDownloader() error = %v", err)
	}

	goMod := readGeneratedFile(t, dir, "go.mod")
	if !strings.Contains(goMod, "module my-project") {
		t.Fatalf("go.mod was not rendered with module name: %q", goMod)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod.tmpl")); !os.IsNotExist(err) {
		t.Fatalf("go.mod.tmpl should not be materialized, err=%v", err)
	}

	mainGo := readGeneratedFile(t, dir, "cmd/api/main.go")
	if !strings.Contains(mainGo, "my-project/internal/home/http") {
		t.Fatalf("imports were not rendered with module name: %q", mainGo)
	}

	accessFile := readGeneratedFile(t, dir, "internal/shared/access.go")
	if count := strings.Count(accessFile, "owner@example.com"); count != 2 {
		t.Fatalf("bootstrap email count = %d, want 2 in allowlists: %q", count, accessFile)
	}

	for _, path := range []string{".air.toml", ".gitignore", "logo.svg", "login.jpeg", "internal/editor/http/register.go", "scripts/_tree_generator.py", "scripts/structure.config.toml"} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be materialized: %v", path, err)
		}
	}

	hookInfo, err := os.Stat(filepath.Join(dir, ".githooks/post-checkout"))
	if err != nil {
		t.Fatalf("expected post-checkout hook to be materialized: %v", err)
	}
	if runtime.GOOS != "windows" && hookInfo.Mode().Perm()&0o700 != 0o700 {
		t.Fatalf("post-checkout permissions = %o, want owner read/write/execute", hookInfo.Mode().Perm())
	}
}

func readGeneratedFile(t *testing.T, dir, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(b)
}
