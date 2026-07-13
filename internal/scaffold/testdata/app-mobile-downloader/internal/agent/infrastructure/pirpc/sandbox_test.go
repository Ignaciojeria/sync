package pirpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeSessionID(t *testing.T) {
	cases := map[string]string{
		"agent-1783013382907611187": "agent-1783013382907611187",
		"plain id":                  "plain-id",
		"weird/with\\slashes":       "weird-with-slashes",
		"":                          "",
		"---":                       "",
		"a.b_c-d":                   "a.b_c-d",
		"café":                      "caf-",
	}
	for in, want := range cases {
		if got := sanitizeSessionID(in); got != want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveCWD_DefaultSandbox(t *testing.T) {
	// Cambiamos el cwd del test a un tempdir para que la creación del
	// sandbox no ensucie el repo.
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	for _, cwd := range []string{"", ".", "./", "  "} {
		got, err := resolveCWD(cwd, "agent-test-001")
		if err != nil {
			t.Fatalf("resolveCWD(%q): %v", cwd, err)
		}
		if !strings.HasSuffix(got, filepath.Join("tmp", "agent-work", "agent-test-001")) {
			t.Errorf("resolveCWD(%q) = %q, esperaba terminar en tmp/agent-work/agent-test-001", cwd, got)
		}
		info, err := os.Stat(got)
		if err != nil {
			t.Fatalf("sandbox no creado: %v", err)
		}
		if !info.IsDir() {
			t.Fatal("sandbox no es un directorio")
		}
	}
}

func TestResolveCWD_RespectsExplicitPath(t *testing.T) {
	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })

	explicit, err := filepath.Abs("/tmp/explicit-cwd-for-pi")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	got, err := resolveCWD(explicit, "agent-test-ignored")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != explicit {
		t.Fatalf("got %q, want %q", got, explicit)
	}
}

func TestResolveCWD_RejectsEmptySessionIDWithDefault(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got, err := resolveCWD("", "")
	if err == nil {
		t.Fatalf("esperaba error para sessionID vacío, obtuve %q", got)
	}
}

func TestResolveCWD_SeedsPiConfig(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	// Simulamos un repo con .pi/extensions/provider.ts
	extDir := filepath.Join(tmp, PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir .pi: %v", err)
	}
	want := []byte("export const provider = {}\n")
	if err := os.WriteFile(filepath.Join(extDir, "provider.ts"), want, 0o644); err != nil {
		t.Fatalf("write provider: %v", err)
	}

	sandbox, err := resolveCWD("", "agent-seed-001")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(sandbox, PiConfigDir, "extensions", "provider.ts"))
	if err != nil {
		t.Fatalf(".pi no sembrado en el sandbox: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contenido copiado = %q, want %q", got, want)
	}
}

func TestSeedPiConfig_Idempotent(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	if err := os.MkdirAll(filepath.Join(tmp, PiConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir .pi: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, PiConfigDir, "config"), []byte("orig"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	// Primera siembra.
	if err := seedPiConfig(dest); err != nil {
		t.Fatalf("seedPiConfig 1: %v", err)
	}
	// El agente modifica la config dentro del sandbox.
	sandboxCfg := filepath.Join(dest, PiConfigDir, "config")
	if err := os.WriteFile(sandboxCfg, []byte("modificado"), 0o644); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
	// Segunda siembra no debe pisar lo del sandbox.
	if err := seedPiConfig(dest); err != nil {
		t.Fatalf("seedPiConfig 2: %v", err)
	}
	got, err := os.ReadFile(sandboxCfg)
	if err != nil {
		t.Fatalf("read sandbox cfg: %v", err)
	}
	if string(got) != "modificado" {
		t.Fatalf("seedPiConfig pisó config existente: %q", got)
	}
}

func TestSeedPiConfig_NoRepoConfig(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	// Sin .pi en el repo no debe ser error.
	if err := seedPiConfig(dest); err != nil {
		t.Fatalf("seedPiConfig sin .pi debe ser no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, PiConfigDir)); !os.IsNotExist(err) {
		t.Fatalf("no debería existir .pi en el sandbox, stat err = %v", err)
	}
}

func TestResolveCWD_AcceptsRelativeExplicitCWD(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	got, err := resolveCWD("./explicit-relative", "agent-test-002")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != "./explicit-relative" {
		t.Fatalf("got %q, want %q", got, "./explicit-relative")
	}
}
