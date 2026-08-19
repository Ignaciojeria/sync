package pirpc

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// extensionsDir devuelve el path al dir de extensions del workspace
// del agente (agents/<agentID>/.pi/extensions). Helper para no
// repetir el patrón en cada test del archivo.
func extensionsDir(root, agentID string) string {
	return filepath.Join(root, AgentsRoot, agentID, PiConfigDir, "extensions")
}

// sandboxedExtensionsDir devuelve el path donde el runner siembra
// las extensions dentro del sandbox de una sesión. En el runtime
// real, el sandbox se crea con seedPiConfig desde
// agents/<agentID>/.pi/, así que termina en <sandbox>/.pi/extensions.
func sandboxedExtensionsDir(sandbox string) string {
	return filepath.Join(sandbox, PiConfigDir, "extensions")
}

func TestDiscoverProjectExtensions_DedupByContent(t *testing.T) {
	// Simula el caso real: el sandbox (cwd del runtime) tiene
	// las extensions sembradas por seedPiConfig, y el cwd del
	// proceso (fallback) apunta al workspace del agente en
	// agents/develop/.pi/extensions. Cuando ambos árboles
	// tienen contenido idéntico, el dedup por md5 deja una sola
	// copia.
	repo := t.TempDir()
	sandbox := t.TempDir()

	providerContent := "export default function (pi) { pi.setModel({}); }"
	writeFile(t, filepath.Join(sandboxedExtensionsDir(sandbox), "provider.ts"), providerContent)
	writeFile(t, filepath.Join(extensionsDir(repo, "develop"), "provider.ts"), providerContent)

	smokeContent := "export default function () { return 'smoke'; }"
	writeFile(t, filepath.Join(sandboxedExtensionsDir(sandbox), "smoke.ts"), smokeContent)
	writeFile(t, filepath.Join(extensionsDir(repo, "develop"), "smoke.ts"), smokeContent)

	prev, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got := discoverProjectExtensions(sandbox, "develop")

	// Esperar 2 paths únicos (provider.ts + smoke.ts), no 4.
	if len(got) != 2 {
		t.Errorf("discoverProjectExtensions() returned %d paths, want 2 (dedup por contenido):\n  %v", len(got), got)
	}

	seen := map[string]bool{}
	for _, p := range got {
		seen[filepath.Base(p)] = true
	}
	if !seen["provider.ts"] || !seen["smoke.ts"] {
		t.Errorf("missing expected extensions, got: %v", got)
	}
}

func TestDiscoverProjectExtensions_KeepsDifferentContent(t *testing.T) {
	// Mismo setup que _DedupByContent pero con contenido
	// distinto: el sandbox tiene provider.ts modificado (ej. una
	// sesión previa lo editó) y el workspace del agente tiene la
	// versión original. En este caso NO se dedupique: el agente
	// quiere ver la versión modificada del sandbox.
	repo := t.TempDir()
	sandbox := t.TempDir()

	writeFile(t, filepath.Join(sandboxedExtensionsDir(sandbox), "provider.ts"),
		"export default function (pi) { pi.setModel({}); /* sandbox */ }")
	writeFile(t, filepath.Join(extensionsDir(repo, "develop"), "provider.ts"),
		"export default function (pi) { pi.setModel({}); /* workspace */ }")

	prev, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got := discoverProjectExtensions(sandbox, "develop")

	if len(got) != 2 {
		t.Errorf("discoverProjectExtensions() returned %d paths, want 2 (contenido distinto):\n  %v", len(got), got)
	}
}

func TestDiscoverProjectExtensions_DedupByPath(t *testing.T) {
	// Si el cwd del runtime y el fallback del cwd del proceso
	// apuntan al mismo dir (caso borde: chdir al workspace del
	// agente y cwd del runtime = sandbox dentro del mismo repo),
	// el dedup por path sigue funcionando.
	shared := t.TempDir()
	writeFile(t, filepath.Join(extensionsDir(shared, "develop"), "provider.ts"), "export default {}")

	// El cwd del runtime coincide con el cwd del proceso, así
	// que ambos roots colapsan al mismo path.
	prev, _ := os.Getwd()
	if err := os.Chdir(shared); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got := discoverProjectExtensions(shared, "develop")

	if len(got) != 1 {
		t.Errorf("expected 1 path (path dedup), got %d: %v", len(got), got)
	}
}
