package application

import "context"

// Runner crea runtimes capaces de hablar con pi para una sesión concreta.
type Runner interface {
	Start(ctx context.Context, spec StartSpec) (Runtime, error)
}

// Runtime representa una sesión viva del agente.
type Runtime interface {
	SessionID() string
	Prompt(ctx context.Context, message string) error
	Steer(ctx context.Context, message string) error
	Abort(ctx context.Context) error
	Subscribe() (<-chan Event, func())
	State() RuntimeState
	Close() error
}

// StartSpec contiene los datos mínimos para levantar una sesión de pi.
type StartSpec struct {
	SessionID string
	CWD       string
	Model     string
	Title     string
	SessionFile string
	// AgentID identifica qué agente corre esta sesión (ej.
	// "develop"). Lo consume pirpc.resolveCWD para sembrar el
	// .pi/ y AGENTS.md desde agents/<AgentID>/. Si viene vacío,
	// pirpc cae a DefaultAgentID() — no se valida contra el
	// registry porque el Manager ya lo normalizó antes.
	AgentID string
	// DisableNativeHonchoTools indica al runner que NO cargue la
	// extensión Honcho de .pi/extensions/ en el spawn. Útil cuando
	// el host ya enruta memoria vía MemoryProvider (Fase C) y
	// quiere evitar doble consumo. Hoy no hay tal extensión en
	// este repo (.pi/extensions/ sólo tiene provider.ts y
	// smoke.ts), así que la flag es forward-compat para cuando
	// aparezca. Si el pi binary suma honcho built-in en el
	// futuro, también habría que filtrar acá; mientras tanto,
	// no-op.
	DisableNativeHonchoTools bool
}

// RuntimeState resume el estado observable del runtime.
type RuntimeState struct {
	Status      string `json:"status"`
	IsStreaming bool   `json:"isStreaming"`
	Model       string `json:"model"`
	LastError   string `json:"lastError,omitempty"`
	Closed      bool   `json:"closed"`
}
