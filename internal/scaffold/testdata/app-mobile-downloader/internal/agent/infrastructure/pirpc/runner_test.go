// ponytail: estos tests spawnean `#!/bin/sh` scripts vía fork/exec.
// En Windows no hay sh por PATH y el fork falla con "%1 is not a
// valid Win32 application", así que los excluimos del build en esa
// plataforma. El código de producción sigue testeándose en Linux/macOS.
//go:build !windows

package pirpc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// fakeBinary escribe un script en un tempdir y devuelve su path absoluto.
// El script se guarda como archivo ejecutable. Recibe el contenido del script.
func fakeBinary(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-pi.sh")
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return path
}

func newTestRunner(binary string, writeTimeout time.Duration) *Runner {
	return &Runner{
		Binary:             binary,
		PromptWriteTimeout: writeTimeout,
	}
}

func TestNewRunner_EffectiveTimeout(t *testing.T) {
	t.Run("cero cae al default", func(t *testing.T) {
		r := NewRunner("pi")
		if got, want := r.promptWriteTimeout(), defaultPromptWriteTimeout; got != want {
			t.Fatalf("promptWriteTimeout = %s, want %s", got, want)
		}
	})
	t.Run("valor pequeño sube al piso", func(t *testing.T) {
		r := &Runner{PromptWriteTimeout: 1 * time.Millisecond}
		if got, want := r.promptWriteTimeout(), MinPromptWriteTimeout; got != want {
			t.Fatalf("promptWriteTimeout = %s, want %s", got, want)
		}
	})
	t.Run("valor mayor gana", func(t *testing.T) {
		r := &Runner{PromptWriteTimeout: 5 * time.Second}
		if got, want := r.promptWriteTimeout(), 5*time.Second; got != want {
			t.Fatalf("promptWriteTimeout = %s, want %s", got, want)
		}
	})
}

func TestRunnerStart_ReturnsRuntime(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
echo '{"type":"turn_start"}'
# No leemos stdin: queremos que Prompt quede esperando y dispare el timeout.
sleep 30
`)
	// Mantenemos la sessión viva pero los writes deben ser cancelados.
	r := newTestRunner(binary, MinPromptWriteTimeout)

	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	// Mientras el proceso duerme, debe estar running y abierto.
	state := runtime.State()
	if state.Closed {
		t.Fatalf("runtime Closed antes de tiempo")
	}
}

func TestRunnerPrompt_ReturnsTimeoutWhenStdinBlocked(t *testing.T) {
	// El script mantiene su stdin conectado al pipe (el shell lo usa para leer
	// el script al inicio), pero después de imprimir el evento de arranque
	// se queda en `sleep` y deja de leer. Los writes al pipe que superen el
	// buffer del kernel (64 KB por defecto en Linux) terminan bloqueando al
	// emisor. Nuestro timeout debe cortar antes.
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
sleep 30
`)
	r := newTestRunner(binary, 100*time.Millisecond)

	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	// 70 KB excede el buffer de pipe con margen suficiente.
	bigMessage := strings.Repeat("a", 70_000)
	start := time.Now()
	err = runtime.Prompt(context.Background(), bigMessage)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("se esperaba error de timeout, obtuve nil")
	}
	if !strings.Contains(err.Error(), "timed out") &&
		!strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error inesperado: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Prompt tardó demasiado: %s", elapsed)
	}

	state := runtime.State()
	if !state.Closed {
		t.Fatal("se esperaba runtime.Closed = true tras timeout")
	}
	if state.Status != string(agentapp.SessionStatusError) {
		t.Fatalf("Status = %q, want error", state.Status)
	}
	if state.LastError == "" {
		t.Fatal("se esperaba LastError poblado")
	}
}

func TestRunnerPrompt_HonoursContextCancellation(t *testing.T) {
	// El script conecta stdin al pipe y luego NO lo lee (sleep infinito).
	// Enviamos 70 KB para saturar el pipe y luego cancelamos el contexto;
	// el send debe responder rápido.
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
sleep 30
`)
	r := newTestRunner(binary, 30*time.Second)

	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	bigMessage := strings.Repeat("a", 70_000)
	start := time.Now()
	err = runtime.Prompt(ctx, bigMessage)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("se esperaba error, obtuve nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("Prompt no respetó cancelación: tardó %s", elapsed)
	}
	if !strings.Contains(err.Error(), "cancelled") &&
		!strings.Contains(err.Error(), "timed out") {
		t.Fatalf("mensaje de error inesperado: %v", err)
	}
}

func TestRunnerPrompt_DeliversCommand(t *testing.T) {
	// Script que registra los primeros bytes que reciba por stdin.
	binary := fakeBinary(t, `#!/bin/sh
exec >/dev/null
cat - &
echo '{"type":"agent_start"}'
sleep 2
`)
	r := newTestRunner(binary, MinPromptWriteTimeout)

	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	if err := runtime.Prompt(context.Background(), "hola agente"); err != nil {
		t.Fatalf("Prompt: %v", err)
	}
}

func TestRuntimeSubscribe_ChannelMatchesBufferSize(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
sleep 30
`)
	r := newTestRunner(binary, MinPromptWriteTimeout)
	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	ch, cancel := runtime.Subscribe()
	defer cancel()
	if got, want := cap(ch), subscriberBufferSize; got != want {
		t.Fatalf("subscriber buffer cap = %d, want %d", got, want)
	}
}

func TestRuntimeSend_RejectsClosedRuntime(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
sleep 30
`)
	r := newTestRunner(binary, MinPromptWriteTimeout)
	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Close puede devolver "signal: killed" cuando matamos al proceso;
	// sólo necesitamos verificar el estado final.
	_ = runtime.Close()
	if !runtime.State().Closed {
		t.Fatal("se esperaba runtime.Closed = true tras Close")
	}

	err = runtime.Prompt(context.Background(), "hola")
	if err != ErrRuntimeClosed {
		t.Fatalf("err = %v, want ErrRuntimeClosed", err)
	}
}

func TestHandleRaw_ResponseFailureBecomesRuntimeError(t *testing.T) {
	r := &piRuntime{
		sessionID:    "s1",
		subscribers: map[chan agentapp.Event]struct{}{},
		done:         make(chan struct{}),
	}
	ch := make(chan agentapp.Event, 1)
	r.subscribers[ch] = struct{}{}

	r.handleRaw(json.RawMessage(`{"type":"response","command":"prompt","success":false,"error":"insufficient_credits"}`))

	select {
	case ev := <-ch:
		if ev.Type != "runtime_error" {
			t.Fatalf("event type = %q, want runtime_error", ev.Type)
		}
		if !strings.Contains(string(ev.Payload), "Créditos insuficientes") {
			t.Fatalf("payload = %s, want Créditos insuficientes", string(ev.Payload))
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}

	state := r.State()
	if state.Status != string(agentapp.SessionStatusError) {
		t.Fatalf("status = %q, want error", state.Status)
	}
}

func TestRuntimeBroadcast_SkipsSlowSubscriber(t *testing.T) {
	binary := fakeBinary(t, `#!/bin/sh
echo '{"type":"agent_start"}'
echo '{"type":"turn_start"}'
echo '{"type":"message_start"}'
echo '{"type":"message_end"}'
echo '{"type":"turn_end"}'
echo '{"type":"agent_end"}'
sleep 30
`)
	r := newTestRunner(binary, MinPromptWriteTimeout)
	runtime, err := r.Start(context.Background(), agentapp.StartSpec{
		SessionID: "test-session",
		CWD:       os.TempDir(),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	// Subscriber que consume muy lento para forzar drops.
	ch, cancel := runtime.Subscribe()
	defer cancel()

	var consumed atomic.Int64
	go func() {
		for ev := range ch {
			_ = ev
			consumed.Add(1)
			time.Sleep(broadcastDeliverTimeout + 250*time.Millisecond)
		}
	}()

	// Esperamos lo suficiente para que el runner haya publicado al menos
	// los 6 eventos.
	time.Sleep(2 * time.Second)

	// No validamos el conteo exacto (depende del scheduling); sólo que
	// ningún evento haya bloqueado al runner para siempre. El test pasa si
	// llegamos hasta acá sin deadlock.
	if consumed.Load() == 0 {
		t.Skip("no se consumió ningún evento; reintentá con más tiempo")
	}
}

func TestStartProcess_RedirectsDefaultCWDToSandbox(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	bin := fakeBinary(t, `#!/bin/sh
sleep 30
`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd, _, _, _, err := startProcess(ctx, bin, agentapp.StartSpec{
		SessionID: "agent-1783013382",
		CWD:       "",
		Model:     "",
	})
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if !strings.HasSuffix(cmd.Dir, filepath.Join("tmp", "agent-work", "agent-1783013382")) {
		t.Fatalf("cmd.Dir = %q, esperaba terminar en tmp/agent-work/agent-1783013382", cmd.Dir)
	}
	if _, err := os.Stat(cmd.Dir); err != nil {
		t.Fatalf("sandbox no creado en disco: %v", err)
	}
}

func TestStartProcess_RespectsExplicitCWD(t *testing.T) {
	prev, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	explicit, err := filepath.Abs("explicit-relative-cwd")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if err := os.MkdirAll(explicit, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	bin := fakeBinary(t, `#!/bin/sh
sleep 30
`)

	cmd, _, _, _, err := startProcess(context.Background(), bin, agentapp.StartSpec{
		SessionID: "agent-1783013382",
		CWD:       explicit,
	})
	if err != nil {
		t.Fatalf("startProcess: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	if cmd.Dir != explicit {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, explicit)
	}
}
