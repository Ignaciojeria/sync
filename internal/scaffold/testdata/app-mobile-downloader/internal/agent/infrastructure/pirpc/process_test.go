package pirpc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverProjectExtensions_FallsBackToAgentWorkspace(t *testing.T) {
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

	// El fallback busca en agents/<agentID>/.pi/extensions del cwd del
	// proceso. Antes vivía en la raíz (.pi/extensions); ahora vive
	// bajo el workspace del agente.
	extDir := filepath.Join(repoRoot, AgentsRoot, "develop", PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir agents/develop/.pi/extensions: %v", err)
	}
	want := filepath.Join(extDir, "preview.ts")
	if err := os.WriteFile(want, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatalf("write extension: %v", err)
	}

	got := discoverProjectExtensions(workspace, "develop")
	if len(got) != 1 || filepath.Clean(got[0]) != filepath.Clean(want) {
		t.Fatalf("discoverProjectExtensions() = %#v, want [%q]", got, want)
	}
}

func TestDiscoverProjectExtensions_AgentIDEmptyFallsBackToDefault(t *testing.T) {
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

	extDir := filepath.Join(repoRoot, AgentsRoot, "develop", PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := filepath.Join(extDir, "provider.ts")
	if err := os.WriteFile(want, []byte("export default {}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got := discoverProjectExtensions(workspace, "")
	if len(got) != 1 || filepath.Clean(got[0]) != filepath.Clean(want) {
		t.Fatalf("discoverProjectExtensions(workspace, \"\") = %#v, want [%q]", got, want)
	}
}

func TestIsHonchoExtensionPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/abs/.pi/extensions/honcho/index.ts", true},
		{"/abs/.pi/extensions/honcho.ts", true},
		{"/abs/.pi/extensions/honcho.js", true},
		{"honcho", true},
		{"/abs/.pi/extensions/provider.ts", false},
		{"/abs/.pi/extensions/preview/index.ts", false},
		{"/abs/honchofalso/index.ts", false}, // matchea basename sin extensión
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isHonchoExtensionPath(tt.path); got != tt.want {
				t.Errorf("isHonchoExtensionPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
