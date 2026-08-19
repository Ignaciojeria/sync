package pirpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	agentapp "lastmile-agents/internal/agent/application"
)

// Tamaño del buffer de cada subscriber. Suficiente para amortiguar picos de
// eventos durante un turn sin que el runner tenga que bloquear al lector
// de stdout (que es quien publica).
const subscriberBufferSize = 256

// Límite superior independiente del contexto para un write a stdin. El kernel
// Linux tiene un pipe de ~64KB; si pi no lee, un write puede bloquearse
// indefinidamente. Forzamos techo para no colgar al handler HTTP.
const defaultPromptWriteTimeout = 10 * time.Second

// MinPromptWriteTimeout es el piso que aceptamos al inyectar un timeout más
// agresivo (tests o callers que quieren fail-fast).
const MinPromptWriteTimeout = 50 * time.Millisecond

// Tiempo máximo para entregar un evento a un subscriber lento antes de
// descartar y avisar.
const broadcastDeliverTimeout = 2 * time.Second

// ErrRuntimeClosed se devuelve cuando el proceso pi ya terminó y alguien
// intenta mandar comandos.
var ErrRuntimeClosed = errors.New("pirpc: runtime closed")

type Runner struct {
	Binary string
	// PromptWriteTimeout ajusta el techo del write a stdin. Si es <=0 se usa
	// defaultPromptWriteTimeout. Se acota inferiormente por
	// MinPromptWriteTimeout para evitar valores absurdos en tests.
	PromptWriteTimeout time.Duration
}

func NewRunner(binary string) *Runner {
	return &Runner{Binary: strings.TrimSpace(binary)}
}

// promptWriteTimeout devuelve el timeout efectivo. Si Runner no tiene valor
// explícito, retorna el default.
func (r *Runner) promptWriteTimeout() time.Duration {
	if r.PromptWriteTimeout <= 0 {
		return defaultPromptWriteTimeout
	}
	if r.PromptWriteTimeout < MinPromptWriteTimeout {
		return MinPromptWriteTimeout
	}
	return r.PromptWriteTimeout
}

func (r *Runner) Start(_ context.Context, spec agentapp.StartSpec) (agentapp.Runtime, error) {
	binary := r.Binary
	if binary == "" {
		binary = "pi"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("pi binary not found: %w", err)
	}

	procCtx, cancel := context.WithCancel(context.Background())
	cmd, stdin, stdout, stderr, err := startProcess(procCtx, binary, spec)
	if err != nil {
		cancel()
		return nil, err
	}

	runtime := &piRuntime{
		sessionID:    spec.SessionID,
		cmd:          cmd,
		stdin:        stdin,
		cancel:       cancel,
		writeTimeout: r.promptWriteTimeout(),
		state: agentapp.RuntimeState{
			Status: string(agentapp.SessionStatusIdle),
			Model:  spec.Model,
		},
		subscribers: map[chan agentapp.Event]struct{}{},
		done:        make(chan struct{}),
		spawnedAt:   time.Now(),
	}
	go runtime.consume(stdout)
	go runtime.consumeStderr(stderr)
	go runtime.wait()
	return runtime, nil
}

type piRuntime struct {
	sessionID    string
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	cancel       context.CancelFunc
	writeTimeout time.Duration

	mu          sync.Mutex
	state       agentapp.RuntimeState
	subscribers map[chan agentapp.Event]struct{}
	waitErr     error
	done        chan struct{}
	spawnedAt   time.Time
}

func (r *piRuntime) SessionID() string { return r.sessionID }

func (r *piRuntime) Prompt(ctx context.Context, message string) error {
	return r.send(ctx, map[string]any{"type": "prompt", "message": strings.TrimSpace(message)})
}

func (r *piRuntime) Steer(ctx context.Context, message string) error {
	return r.send(ctx, map[string]any{"type": "steer", "message": strings.TrimSpace(message)})
}

func (r *piRuntime) Abort(ctx context.Context) error {
	return r.send(ctx, map[string]any{"type": "abort"})
}

func (r *piRuntime) Subscribe() (<-chan agentapp.Event, func()) {
	ch := make(chan agentapp.Event, subscriberBufferSize)
	r.mu.Lock()
	r.subscribers[ch] = struct{}{}
	r.mu.Unlock()
	cancel := func() {
		r.mu.Lock()
		delete(r.subscribers, ch)
		r.mu.Unlock()
	}
	return ch, cancel
}

func (r *piRuntime) State() agentapp.RuntimeState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *piRuntime) Close() error {
	r.cancel()
	_ = r.stdin.Close()
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
	<-r.done
	return r.waitErr
}

// send escribe un comando JSON al stdin del proceso pi. El write está limitado
// por ctx (cancelación) y por defaultPromptWriteTimeout (techo cuando ctx no
// tiene deadline) para que un pi atascado no cuelgue al handler HTTP. Si el
// timeout salta, el runtime se marca como cerrado y se mata el proceso, así
// el próximo prompt dispara un spawn limpio.
func (r *piRuntime) send(ctx context.Context, command map[string]any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if r.state.Closed {
		r.mu.Unlock()
		return ErrRuntimeClosed
	}
	r.mu.Unlock()

	payload, err := json.Marshal(command)
	if err != nil {
		return err
	}

	timeout := r.writeTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}

	done := make(chan error, 1)
	go func() {
		_, werr := io.WriteString(r.stdin, string(payload)+"\n")
		done <- werr
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		r.killAndClose(fmt.Sprintf("prompt cancelled: %v", ctx.Err()))
		return fmt.Errorf("pirpc send cancelled: %w", ctx.Err())
	case <-timer.C:
		r.killAndClose(fmt.Sprintf("stdin write timeout after %s", timeout))
		return fmt.Errorf("pirpc send timed out after %s", timeout)
	case werr := <-done:
		return werr
	}
}

// killAndClose apaga el proceso pi y marca el runtime como cerrado para que
// el próximo Manager.Prompt dispare un respawn limpio. Idempotente.
func (r *piRuntime) killAndClose(reason string) {
	r.mu.Lock()
	wasClosed := r.state.Closed
	if !wasClosed {
		r.state.Closed = true
		r.state.IsStreaming = false
		r.state.LastError = reason
		r.state.Status = string(agentapp.SessionStatusError)
	}
	r.mu.Unlock()

	if !wasClosed {
		slog.Warn("pirpc: killing runtime",
			"session_id", r.sessionID,
			"reason", reason,
			"uptime", time.Since(r.spawnedAt).String(),
		)
		go r.broadcastLocal("runtime_exit", map[string]any{"reason": reason})
	}

	r.cancel()
	_ = r.stdin.Close()
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}

func (r *piRuntime) consume(stdout io.Reader) {
	err := readJSONL(stdout, func(raw json.RawMessage) error {
		r.handleRaw(raw)
		return nil
	})
	if err != nil {
		r.broadcastLocal("runtime_error", map[string]any{"message": err.Error()})
		r.setError(err.Error())
	}
}

func (r *piRuntime) consumeStderr(stderr io.Reader) {
	reader := bufio.NewReader(stderr)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if len(bytes.TrimSpace(line)) > 0 {
				message := string(line)
				if normalized, ok := normalizeProviderRuntimeError(message); ok {
					r.broadcastLocal("runtime_error", map[string]any{"message": normalized})
					r.setError(normalized)
				} else {
					r.broadcastLocal("stderr", map[string]any{"message": message})
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (r *piRuntime) wait() {
	err := r.cmd.Wait()
	r.mu.Lock()
	state := r.state
	state.Closed = true
	state.IsStreaming = false
	if err != nil && strings.TrimSpace(state.LastError) == "" {
		state.LastError = err.Error()
		state.Status = string(agentapp.SessionStatusError)
		go r.broadcastLocal("runtime_exit", map[string]any{"message": err.Error()})
	} else if state.Status != string(agentapp.SessionStatusError) {
		state.Status = string(agentapp.SessionStatusIdle)
	}
	r.state = state
	r.waitErr = err
	subscribers := make([]chan agentapp.Event, 0, len(r.subscribers))
	for ch := range r.subscribers {
		subscribers = append(subscribers, ch)
		delete(r.subscribers, ch)
	}
	r.mu.Unlock()
	for _, ch := range subscribers {
		close(ch)
	}
	close(r.done)
}

func (r *piRuntime) handleRaw(raw json.RawMessage) {
	var header struct {
		Type    string          `json:"type"`
		Message json.RawMessage `json:"message"`
		Error   string          `json:"error"`
		Status  int             `json:"status"`
		Detail  string          `json:"detail"`
		Success bool            `json:"success"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		r.broadcastLocal("runtime_error", map[string]any{"message": err.Error()})
		r.setError(err.Error())
		return
	}
	if header.Type == "response" && !header.Success {
		message := firstNonEmpty(strings.TrimSpace(header.Error), strings.TrimSpace(header.Detail), strings.TrimSpace(string(header.Message)))
		if normalized, ok := normalizeProviderRuntimeError(string(raw)); ok {
			message = normalized
		}
		if strings.TrimSpace(message) == "" {
			message = "El agente rechazó la operación."
		}
		header.Type = "runtime_error"
		raw = mustMarshal(map[string]any{"type": "runtime_error", "message": message})
		r.setError(message)
	}
	if header.Type == "runtime_error" {
		if normalized, ok := normalizeProviderRuntimeError(string(raw)); ok {
			raw = mustMarshal(map[string]any{"type": "runtime_error", "message": normalized})
			r.setError(normalized)
		}
	}
	r.updateState(header.Type)
	r.broadcastEvent(header.Type, raw)
}

func (r *piRuntime) updateState(eventType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	state := r.state
	switch eventType {
	case "agent_start", "turn_start", "message_start", "tool_execution_start":
		state.Status = string(agentapp.SessionStatusRunning)
		state.IsStreaming = true
	case "agent_end", "turn_end":
		state.Status = string(agentapp.SessionStatusIdle)
		state.IsStreaming = false
	case "runtime_error", "stderr":
		state.Status = string(agentapp.SessionStatusError)
	}
	r.state = state
}

func (r *piRuntime) setError(message string) {
	r.mu.Lock()
	state := r.state
	state.Status = string(agentapp.SessionStatusError)
	state.IsStreaming = false
	state.LastError = strings.TrimSpace(message)
	r.state = state
	r.mu.Unlock()
}

func (r *piRuntime) broadcastLocal(eventType string, payload any) {
	r.broadcastEvent(eventType, mustMarshal(payload))
}

func (r *piRuntime) broadcastEvent(eventType string, payload json.RawMessage) {
	event := agentapp.Event{
		SessionID: r.sessionID,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	r.mu.Lock()
	subscribers := make([]chan agentapp.Event, 0, len(r.subscribers))
	for ch := range r.subscribers {
		subscribers = append(subscribers, ch)
	}
	r.mu.Unlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		case <-time.After(broadcastDeliverTimeout):
			slog.Warn("pirpc: dropping event for slow subscriber",
				"session_id", r.sessionID,
				"event_type", eventType,
				"subscriber_chan_cap", cap(ch),
			)
		}
	}
}

func mustMarshal(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func normalizeProviderRuntimeError(message string) (string, bool) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "", false
	}

	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Title   string `json:"title"`
		Status  int    `json:"status"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if isInsufficientCredits(payload.Error, payload.Message, payload.Detail, payload.Title, payload.Status) {
			return "Créditos insuficientes en el proveedor/modelo configurado.", true
		}
	}

	if isInsufficientCredits("", trimmed, trimmed, trimmed, 0) {
		return "Créditos insuficientes en el proveedor/modelo configurado.", true
	}
	return "", false
}

func isInsufficientCredits(errorCode, message, detail, title string, status int) bool {
	if status == 402 {
		return true
	}
	text := strings.ToLower(strings.Join([]string{errorCode, message, detail, title}, " "))
	return strings.Contains(text, "insufficient_credits") ||
		strings.Contains(text, "insufficient credits") ||
		strings.Contains(text, "créditos insuficientes") ||
		strings.Contains(text, "payment required") ||
		strings.Contains(text, `"status": 402`) ||
		strings.Contains(text, `"status":402`)
}
