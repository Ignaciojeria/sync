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
	SessionID   string
	CWD         string
	Model       string
	Title       string
	SessionFile string
}

// RuntimeState resume el estado observable del runtime.
type RuntimeState struct {
	Status      string `json:"status"`
	IsStreaming bool   `json:"isStreaming"`
	Model       string `json:"model"`
	LastError   string `json:"lastError,omitempty"`
	Closed      bool   `json:"closed"`
}
