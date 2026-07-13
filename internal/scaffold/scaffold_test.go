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
	if !strings.Contains(mainGo, "my-project/internal/home/http") || !strings.Contains(mainGo, "my-project/internal/agent/http") {
		t.Fatalf("imports were not rendered with module name: %q", mainGo)
	}

	accessFile := readGeneratedFile(t, dir, "internal/shared/access.go")
	if count := strings.Count(accessFile, "owner@example.com"); count != 2 {
		t.Fatalf("bootstrap email count = %d, want 2 in allowlists: %q", count, accessFile)
	}

	for _, path := range []string{
		".air.toml",
		".gitignore",
		".githooks/post-checkout",
		"AGENTS.md",
		"design/ocean/DESIGN.md",
		"internal/gateway/http/balance.go",
		"internal/design/http/register.go",
		"internal/editor/http/register.go",
		"internal/agent/http/register.go",
		"internal/agent/http/support.go",
		"doc/agent-runtime.md",
		"scripts/run-api.sh",
		"scripts/_tree_generator.py",
		"scripts/structure.config.toml",
		"wede.config.json",
		"workspaces.yaml",
		"mutagen.yml",
		".pi/extensions/provider.ts",
		".pi/extensions/smoke.ts",
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Fatalf("expected %s to be materialized: %v", path, err)
		}
	}

	mutagenYAML := readGeneratedFile(t, dir, "mutagen.yml")
	if strings.Contains(mutagenYAML, "scaffoldxd1") || !strings.Contains(mutagenYAML, "my-project") {
		t.Fatalf("mutagen.yml was not rendered with project name: %q", mutagenYAML)
	}

	workspacesYAML := readGeneratedFile(t, dir, "workspaces.yaml")
	if strings.Contains(workspacesYAML, "scaffoldxd1") || !strings.Contains(workspacesYAML, "slug: my-project") {
		t.Fatalf("workspaces.yaml was not rendered with project name: %q", workspacesYAML)
	}

	for _, path := range []string{
		".git/config",
		".einar/config.json",
		"tmp",
		"bin/bff",
		"api.exe",
		".air.log",
		".env",
		"mutagen.yml.lock",
	} {
		if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be excluded, err=%v", path, err)
		}
	}

	hookInfo, err := os.Stat(filepath.Join(dir, ".githooks/post-checkout"))
	if err != nil {
		t.Fatalf("expected post-checkout hook to be materialized: %v", err)
	}
	if runtime.GOOS != "windows" && hookInfo.Mode().Perm()&0o700 != 0o700 {
		t.Fatalf("post-checkout permissions = %o, want owner read/write/execute", hookInfo.Mode().Perm())
	}

	runAPIInfo, err := os.Stat(filepath.Join(dir, "scripts/run-api.sh"))
	if err != nil {
		t.Fatalf("expected run-api.sh to be materialized: %v", err)
	}
	if runtime.GOOS != "windows" && runAPIInfo.Mode().Perm()&0o700 != 0o700 {
		t.Fatalf("run-api.sh permissions = %o, want owner read/write/execute", runAPIInfo.Mode().Perm())
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
