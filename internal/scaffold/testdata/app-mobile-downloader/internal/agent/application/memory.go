package application

import (
	"context"
	"time"
)

// MemoryProvider es la interfaz que el Manager usa para enriquecer los
// prompts con contexto persistente entre sesiones y para persistir lo que
// dijo el agente. Vive en application/ por inversión de dependencias:
// la implementación concreta (Honcho, etc.) vive en infrastructure/.
//
// El contrato tiene tres métodos. EnsurePeers es idempotente y barato de
// llamar: el Manager lo invoca antes de Recall/Remember sin saber si el
// provider es real o noop. Esto deja al adapter libre de resolver su
// propio keying (workspaces, peers, sessions) sin filtrar detalles al
// Manager.
type MemoryProvider interface {
	// EnsurePeers garantiza que el workspace, los peers y la session
	// existen en el backend de memoria. Es idempotente: llamadas
	// repetidas con la misma key son no-ops después de la primera
	// creación exitosa. Devuelve error sólo si falla algo recuperable;
	// un backend caído no debe panicar — sólo hacer que Recall/
	// Remember fallen con su error.
	EnsurePeers(ctx context.Context, key MemoryKey) error

	// Recall devuelve contexto relevante al query para inyectar antes
	// de un prompt. El texto retornado ya viene formateado y limitado
	// al budget de tokens configurado en el provider. Un Recall
	// vacío (sin matches) debe devolver MemoryContext con Text="" —
	// NO error — para que el caller decida si steer o no.
	Recall(ctx context.Context, key MemoryKey, query string) (MemoryContext, error)

	// Remember persiste los mensajes de un turno. Es seguro llamar
	// con slices vacíos (no-op). El provider es responsable de
	// batchear, truncar y deduplicar según los límites del backend.
	Remember(ctx context.Context, key MemoryKey, msgs []MemoryMessage) error
}

// MemoryKey identifica el scope de la memoria. Mapea 1:1 al modelo de
// Honcho: WorkspaceID → workspace, SessionID → session, AgentID/UserEmail
// → peers. AgentID queda opcional hasta que Fase 1 del plan
// developer-teams introduzca multi-agente; mientras tanto, el adapter usa
// SessionID como proxy.
type MemoryKey struct {
	WorkspaceID string
	SessionID   string
	UserEmail   string
	AgentID     string
}

// MemoryMessage es un par user/assistant a persistir al final de un turno.
type MemoryMessage struct {
	Role      string    // "user" o "assistant"
	Text      string    // contenido textual; el provider trunca si excede límites
	CreatedAt time.Time // Honcho acepta created_at para preservar orden
}

// MemoryContext es lo que Recall devuelve. Text ya viene formateado
// para inyectar (sin preámbulos tipo "Memoria relevante:"; eso lo agrega
// el caller). TokensUsed es aproximado — el Manager lo usa para logging
// y métricas, no para budgeting client-side.
type MemoryContext struct {
	Text       string
	TokensUsed int
}

// noopProvider es el default cuando HONCHO_ENABLED=false. Garantiza que
// el Manager funcione idéntico al actual: ni Recall inyecta nada, ni
// Remember persiste nada, ni EnsurePeers hace I/O. Existe para que el
// wiring en cmd/api siempre pueda inyectar un provider no-nil.
type noopProvider struct{}

// Compile-time check: noopProvider implementa MemoryProvider.
var _ MemoryProvider = noopProvider{}

func (noopProvider) EnsurePeers(context.Context, MemoryKey) error {
	return nil
}

func (noopProvider) Recall(context.Context, MemoryKey, string) (MemoryContext, error) {
	return MemoryContext{}, nil
}

func (noopProvider) Remember(context.Context, MemoryKey, []MemoryMessage) error {
	return nil
}
