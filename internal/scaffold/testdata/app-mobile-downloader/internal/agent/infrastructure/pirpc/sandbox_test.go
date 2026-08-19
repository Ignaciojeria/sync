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

// withTmpwd cambia el cwd del proceso a un tempdir y restaura al
// terminar. Helper compartido por los tests que necesitan simular
// un repo desde un directorio limpio (porque la siembra busca
// paths relativos al cwd).
func withTmpwd(t *testing.T) string {
	t.Helper()
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return tmp
}

func TestResolveCWD_DefaultSandbox(t *testing.T) {
	withTmpwd(t)

	for _, cwd := range []string{"", ".", "./", "  "} {
		got, err := resolveCWD(cwd, "agent-test-001", "develop")
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
	got, err := resolveCWD(explicit, "agent-test-ignored", "develop")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != explicit {
		t.Fatalf("got %q, want %q", got, explicit)
	}
}

func TestResolveCWD_RejectsEmptySessionIDWithDefault(t *testing.T) {
	withTmpwd(t)

	got, err := resolveCWD("", "", "develop")
	if err == nil {
		t.Fatalf("esperaba error para sessionID vacío, obtuve %q", got)
	}
}

func TestResolveCWD_SeedsPiConfig(t *testing.T) {
	tmp := withTmpwd(t)

	// Simulamos un workspace de agente con .pi/extensions/provider.ts
	agentDir := filepath.Join(tmp, AgentsRoot, "develop")
	extDir := filepath.Join(agentDir, PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir agent .pi: %v", err)
	}
	want := []byte("export const provider = {}\n")
	if err := os.WriteFile(filepath.Join(extDir, "provider.ts"), want, 0o644); err != nil {
		t.Fatalf("write provider: %v", err)
	}

	sandbox, err := resolveCWD("", "agent-seed-001", "develop")
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

func TestResolveCWD_SeedsAgentsDoc(t *testing.T) {
	tmp := withTmpwd(t)

	// Workspace del agente con AGENTS.md propio
	agentDir := filepath.Join(tmp, AgentsRoot, "develop")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("mkdir agent: %v", err)
	}
	want := []byte("# AGENTS — develop\n\nReglas específicas del develop.\n")
	if err := os.WriteFile(filepath.Join(agentDir, "AGENTS.md"), want, 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	sandbox, err := resolveCWD("", "agent-seed-002", "develop")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(sandbox, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md no sembrado en el sandbox: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contenido copiado = %q, want %q", got, want)
	}
}

func TestResolveCWD_DoesNotSeedRootAgentsMd(t *testing.T) {
	tmp := withTmpwd(t)

	// AGENTS.md raíz: debe quedar en la raíz, NO dentro del sandbox.
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte("# raíz\n"), 0o644); err != nil {
		t.Fatalf("write raíz AGENTS.md: %v", err)
	}
	// Workspace del agente sin AGENTS.md (degradado pero legal).
	if err := os.MkdirAll(filepath.Join(tmp, AgentsRoot, "develop"), 0o755); err != nil {
		t.Fatalf("mkdir agent: %v", err)
	}

	sandbox, err := resolveCWD("", "agent-seed-003", "develop")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(sandbox, "AGENTS.md"))
	if err == nil {
		t.Fatalf("no se debería haber copiado AGENTS.md al sandbox, pero está: %q", got)
	}
	if !os.IsNotExist(err) {
		t.Fatalf("error inesperado leyendo sandbox AGENTS.md: %v", err)
	}
}

func TestResolveCWD_AgentIDEmptyFallsBackToDefault(t *testing.T) {
	tmp := withTmpwd(t)

	// Workspace del default agent (develop) con .pi
	extDir := filepath.Join(tmp, AgentsRoot, "develop", PiConfigDir, "extensions")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte("export const provider = {}\n")
	if err := os.WriteFile(filepath.Join(extDir, "provider.ts"), want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	sandbox, err := resolveCWD("", "agent-default-001", "")
	if err != nil {
		t.Fatalf("resolveCWD con agentID vacío: %v", err)
	}

	// Debe haber sembrado desde agents/develop/.pi
	got, err := os.ReadFile(filepath.Join(sandbox, PiConfigDir, "extensions", "provider.ts"))
	if err != nil {
		t.Fatalf(".pi no sembrado al caer al default: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contenido = %q, want %q", got, want)
	}
}

func TestSeedPiConfig_Idempotent(t *testing.T) {
	tmp := withTmpwd(t)

	if err := os.MkdirAll(filepath.Join(tmp, AgentsRoot, "develop", PiConfigDir), 0o755); err != nil {
		t.Fatalf("mkdir .pi: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, AgentsRoot, "develop", PiConfigDir, "config"), []byte("orig"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	// Primera siembra.
	if err := seedPiConfig(dest, "develop"); err != nil {
		t.Fatalf("seedPiConfig 1: %v", err)
	}
	// El agente modifica la config dentro del sandbox.
	sandboxCfg := filepath.Join(dest, PiConfigDir, "config")
	if err := os.WriteFile(sandboxCfg, []byte("modificado"), 0o644); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
	// Segunda siembra no debe pisar lo del sandbox.
	if err := seedPiConfig(dest, "develop"); err != nil {
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
	tmp := withTmpwd(t)

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	// Sin .pi en el workspace del agente no debe ser error.
	if err := seedPiConfig(dest, "develop"); err != nil {
		t.Fatalf("seedPiConfig sin .pi debe ser no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, PiConfigDir)); !os.IsNotExist(err) {
		t.Fatalf("no debería existir .pi en el sandbox, stat err = %v", err)
	}
}

func TestSeedAgentsDoc_CopiesToSandbox(t *testing.T) {
	tmp := withTmpwd(t)

	if err := os.MkdirAll(filepath.Join(tmp, AgentsRoot, "develop"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := []byte("# develop\n")
	if err := os.WriteFile(filepath.Join(tmp, AgentsRoot, "develop", "AGENTS.md"), want, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := seedAgentsDoc(dest, "develop"); err != nil {
		t.Fatalf("seedAgentsDoc: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md no copiado: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contenido = %q, want %q", got, want)
	}
}

func TestSeedAgentsDoc_Idempotent(t *testing.T) {
	tmp := withTmpwd(t)

	if err := os.MkdirAll(filepath.Join(tmp, AgentsRoot, "develop"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, AgentsRoot, "develop", "AGENTS.md"), []byte("orig"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}

	if err := seedAgentsDoc(dest, "develop"); err != nil {
		t.Fatalf("seedAgentsDoc 1: %v", err)
	}
	// Modificamos dentro del sandbox; la segunda siembra no debe pisar.
	if err := os.WriteFile(filepath.Join(dest, "AGENTS.md"), []byte("modificado"), 0o644); err != nil {
		t.Fatalf("write sandbox: %v", err)
	}
	if err := seedAgentsDoc(dest, "develop"); err != nil {
		t.Fatalf("seedAgentsDoc 2: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "modificado" {
		t.Fatalf("seedAgentsDoc pisó AGENTS.md existente: %q", got)
	}
}

func TestSeedAgentsDoc_NoRepoFile(t *testing.T) {
	tmp := withTmpwd(t)

	dest := filepath.Join(tmp, "sandbox")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	// Sin AGENTS.md en el workspace no debe ser error.
	if err := seedAgentsDoc(dest, "develop"); err != nil {
		t.Fatalf("seedAgentsDoc sin AGENTS.md debe ser no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("no debería existir AGENTS.md en el sandbox, stat err = %v", err)
	}
}

func TestResolveCWD_AcceptsRelativeExplicitCWD(t *testing.T) {
	withTmpwd(t)

	got, err := resolveCWD("./explicit-relative", "agent-test-002", "develop")
	if err != nil {
		t.Fatalf("resolveCWD: %v", err)
	}
	if got != "./explicit-relative" {
		t.Fatalf("got %q, want %q", got, "./explicit-relative")
	}
}

// ponytail: regression para el bug del merge multi-agente.
// Cuando el CWD es la raíz del repo (donde antes vivía .pi/ y
// hoy NO vive porque se movió a agents/<id>/.pi/), pi corría
// sin config: catálogo de modelos incompleto + sin API key.
// El fix: si el CWD contiene el workspace del agente con .pi/,
// redirigir al workspace del agente.
func TestResolveCWD_RedirectsToAgentWorkspace(t *testing.T) {
	repo := t.TempDir()
	agentWS := filepath.Join(repo, "agents", "develop")
	if err := os.MkdirAll(filepath.Join(agentWS, ".pi", "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentWS, "AGENTS.md"), []byte("agent rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := resolveCWD(repo, "sess-1", "develop")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(agentWS)
	if got != want {
		t.Fatalf("got %q, want %q. El CWD debería redirigirse al workspace del agente cuando contiene agents/<id>/.pi/",
			got, want)
	}
}

// Si el CWD no contiene el workspace del agente, NO redirigir.
func TestResolveCWD_NoRedirectWhenNoAgentWorkspace(t *testing.T) {
	repo := t.TempDir()
	got, err := resolveCWD(repo, "sess-1", "develop")
	if err != nil {
		t.Fatal(err)
	}
	if got != repo {
		t.Fatalf("got %q, want %q (sin redirect)", got, repo)
	}
}

// Si el workspace del agente existe pero NO tiene .pi/ adentro,
// NO redirigir (no hay nada que ganar cambiando de directorio).
func TestResolveCWD_NoRedirectWhenNoAgentPi(t *testing.T) {
	repo := t.TempDir()
	agentWS := filepath.Join(repo, "agents", "develop")
	if err := os.MkdirAll(agentWS, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sin .pi/ dentro
	got, err := resolveCWD(repo, "sess-1", "develop")
	if err != nil {
		t.Fatal(err)
	}
	if got != repo {
		t.Fatalf("got %q, want %q (sin .pi/ no hay redirect)", got, repo)
	}
}
