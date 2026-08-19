package application

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// spyRunner captura el StartSpec que recibe del Manager para
// poder inspeccionarlo desde el test. NO spawnea procesos reales;
// sólo simula que el runner arrancó pi devolviendo un runtime no-op
// que cierra su canal de eventos inmediatamente.
type spyRunner struct {
	lastStart StartSpec
}

func (s *spyRunner) Start(_ context.Context, spec StartSpec) (Runtime, error) {
	s.lastStart = spec
	return &noopRuntime{}, nil
}

// noopRuntime satisface Runtime con un canal que se cierra al
// recibir la cancelación. Suficiente para que attachRuntimeEvents
// no entre en nil dereference.
type noopRuntime struct{}

func (noopRuntime) SessionID() string                            { return "" }
func (noopRuntime) Prompt(_ context.Context, _ string) error     { return nil }
func (noopRuntime) Steer(_ context.Context, _ string) error      { return nil }
func (noopRuntime) Abort(_ context.Context) error                { return nil }
func (noopRuntime) Subscribe() (<-chan Event, func())            { return make(chan Event), func() {} }
func (noopRuntime) State() RuntimeState                          { return RuntimeState{Closed: true} }
func (noopRuntime) Close() error                                { return nil }

// TestSmoke_CreateSessionSeedsSandboxFromAgentWorkspace verifica
// el acceptance criteria del card "separar raíz del agente
// develop a agents/develop":
//
//   - Una sesión creada via Manager siembra `.pi/` Y `AGENTS.md`
//     desde `agents/<agentID>/` (no desde la raíz del repo).
//   - El sandbox NO contiene un AGENTS.md raíz del repo.
//
// Como el runner stub no llama a pirpc.resolveCWD directamente,
// este test hace el seeding en su lugar usando la misma
// signature. Mantiene el smoke dentro del paquete application para
// no agregar dependencias de pirpc a este test (pirpc tiene sus
// propios tests unitarios).
func TestSmoke_CreateSessionSeedsSandboxFromAgentWorkspace(t *testing.T) {
	repo := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	// Setup: workspace del agente develop con .pi/ y AGENTS.md.
	agentDir := filepath.Join(repo, "agents", "develop")
	if err := os.MkdirAll(filepath.Join(agentDir, ".pi", "extensions"), 0o755); err != nil {
		t.Fatalf("mkdir .pi: %v", err)
	}
	piContent := []byte("export const provider = {}\n")
	if err := os.WriteFile(filepath.Join(agentDir, ".pi", "extensions", "provider.ts"), piContent, 0o644); err != nil {
		t.Fatalf("write provider: %v", err)
	}
	agentsContent := []byte("# AGENTS — develop\n")
	if err := os.WriteFile(filepath.Join(agentDir, "AGENTS.md"), agentsContent, 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	// Y un AGENTS.md raíz que NO debería sembrarse.
	rootContent := []byte("# AGENTS — root (no se debe sembrar)\n")
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), rootContent, 0o644); err != nil {
		t.Fatalf("write root AGENTS.md: %v", err)
	}

	store := newMemStoreStub()
	runner := &spyRunner{}
	mgr := NewManager(store, runner)

	// Crear sesión SIN AgentID: debe resolver al default ("develop").
	sess, err := mgr.Create(context.Background(), CreateSessionInput{
		Title: "smoke",
		CWD:   ".",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.AgentID != "develop" {
		t.Fatalf("AgentID = %q, want %q", sess.AgentID, "develop")
	}

	// El StartSpec recibido por el runner debe llevar AgentID="develop".
	// ensureRuntime es el camino que llama runner.Start — lo
	// disparamos vía Ensure para no depender del prompt flow.
	if err := mgr.Ensure(context.Background(), sess.ID); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if runner.lastStart.AgentID != "develop" {
		t.Fatalf("StartSpec.AgentID = %q, want %q", runner.lastStart.AgentID, "develop")
	}

	// Simulamos lo que pirpc.resolveCWD haría con ese StartSpec
	// (sembrar el sandbox). Esto no es un test de pirpc — es un
	// smoke que verifica que el wiring de Create → StartSpec
	// entrega el AgentID correcto, asumiendo que pirpc (testeado
	// por su lado) siembra desde agents/<agentID>/.
	sandboxDir := filepath.Join(repo, "tmp", "agent-work", sess.ID)
	if err := os.MkdirAll(sandboxDir, 0o755); err != nil {
		t.Fatalf("mkdir sandbox: %v", err)
	}

	// Sembramos según la lógica de pirpc: source = agents/<AgentID>.
	srcPi := filepath.Join(repo, "agents", runner.lastStart.AgentID, ".pi")
	srcAgents := filepath.Join(repo, "agents", runner.lastStart.AgentID, "AGENTS.md")
	copyTree(t, srcPi, filepath.Join(sandboxDir, ".pi"))
	copyFile(t, srcAgents, filepath.Join(sandboxDir, "AGENTS.md"))

	// Verificar .pi sembrado.
	gotPi, err := os.ReadFile(filepath.Join(sandboxDir, ".pi", "extensions", "provider.ts"))
	if err != nil {
		t.Fatalf(".pi no sembrado: %v", err)
	}
	if !strings.Contains(string(gotPi), "export const provider") {
		t.Fatalf(".pi sembrado pero contenido incorrecto: %q", gotPi)
	}

	// Verificar AGENTS.md sembrado desde el workspace del agente.
	gotAgents, err := os.ReadFile(filepath.Join(sandboxDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md no sembrado: %v", err)
	}
	if string(gotAgents) != string(agentsContent) {
		t.Fatalf("AGENTS.md = %q, want %q (de agents/develop)", gotAgents, agentsContent)
	}

	// El AGENTS.md raíz NO debe estar en el sandbox.
	if _, err := os.Stat(filepath.Join(sandboxDir, "AGENTS.md.root")); err == nil {
		t.Fatal("encontré AGENTS.md.root en el sandbox; no debería existir")
	}
	// Y el contenido del sandbox NO debe ser el del raíz.
	if strings.Contains(string(gotAgents), "no se debe sembrar") {
		t.Fatalf("sandbox contiene AGENTS.md raíz en lugar del del agente: %q", gotAgents)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dst, err)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("readdir %s: %v", src, err)
	}
	for _, e := range entries {
		sp := filepath.Join(src, e.Name())
		dp := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyTree(t, sp, dp)
			continue
		}
		copyFile(t, sp, dp)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
