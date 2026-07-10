package pirpc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectExtensions_FallsBackToRepoRootPiDir(t *testing.T) {
	prev, _ := os.Getwd()
	repoRoot := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	extDir := filepath.Join(repoRoot, PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir .pi/extensions: %v", err)
	}
	want := filepath.Join(extDir, "preview.ts")
	if err := os.WriteFile(want, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatalf("write extension: %v", err)
	}

	got := discoverProjectExtensions(workspace)
	if len(got) != 1 || filepath.Clean(got[0]) != filepath.Clean(want) {
		t.Fatalf("discoverProjectExtensions() = %#v, want [%q]", got, want)
	}
}
