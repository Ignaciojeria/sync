---
type: backlog/card
title: Enrutar prompts del agente por Honcho para reducir tokens
description: MemoryProvider opt-in (HONCHO_ENABLED) que inyecta contexto
  relevante de Honcho antes de cada prompt, en modo steer.
status: done
priority: P2
timestamp: 2026-07-19T01:03:13Z
source: user
tags: [agent, honcho, memory, tokens]
---

# Enrutar prompts del agente por Honcho para reducir tokens

Hoy el RPC del agente (`internal/agent/infrastructure/pirpc/process.go`)
inyecta **todas** las extensiones de `.pi/extensions` vía `-e` en cada
spawn, inflando el system prompt de cada turno. Además no hay memoria
persistente entre sesiones, así que pi repite el mismo contexto (reglas
del repo, skills, identidad del usuario) en cada prompt.

Esta tarjeta ata la Fase 2 del plan `doc/agent-teams-plan.md` (Honcho
como `MemoryProvider`) al problema concreto de consumo de tokens,
decidiendo las preguntas que el plan deja abiertas.

# Acceptance Criteria

- [x] Existe `MemoryProvider` en `internal/agent/application/` con
      `Recall(ctx, key, query) (Context, error)` y
      `Remember(ctx, key, msg) error`.
- [x] Default `noopProvider{}` cuando `HONCHO_ENABLED` no está set.
- [x] Adapter `internal/agent/infrastructure/honcho/` que mapea
      `Workspace → workspace`, `Agent → peer`, y usa `peer.context()`
      para Recall + `peer.message()` para Remember.
- [x] Inyección en `manager.PromptRequest` entre `resolvePromptMessage`
      y `runtime.Prompt`, en modo `runtime.Steer` (no como contenido
      del usuario).
- [x] Cuando `HONCHO_ENABLED=true`, se apagan los tools nativos de pi
      (`honcho_search`, `honcho_chat`, `honcho_remember`) para evitar
      doble consumo. Mecanismo a definir (flag `--no-honcho-tools` o
      equivalente en `pirpc.StartSpec`).
- [x] Flush `Remember()` con pares user/assistant al recibir
      `agent_end` (no por prompt individual).
- [x] Keying aislado por `(workspaceID, agentID)` + tests de
      aislamiento entre agentes.
- [x] `Recall` corre dentro de `callCtx` con timeout corto (~2s) para
      no penalizar UX si Honcho está lento.
- [x] Top-k por similitud con budget de tokens configurable (default
      ~1k) para no desbalancear el system prompt.
- [x] `go test ./...` pasa y los tests cubren: noop default, adapter
      mock con `peer.context`, aislamiento entre keys, timeout.

# Plan

## Fase A — Interfaz y no-op (PR chico, sin Honcho todavía)

1. Crear `internal/agent/application/memory.go` con:
   ```go
   type MemoryKey struct { WorkspaceID, AgentID string }
   type MemoryMessage struct { Role, Text string }
   type MemoryContext struct { Items []MemoryItem; TokensUsed int }
   type MemoryProvider interface {
       Recall(ctx context.Context, key MemoryKey, query string) (MemoryContext, error)
       Remember(ctx context.Context, key MemoryKey, msgs []MemoryMessage) error
   }
   type noopProvider struct{}
   ```
2. Tests unitarios del noopProvider.
3. Wiring en `cmd/api/main.go`: `HONCHO_ENABLED` decide entre
   `noopProvider{}` y el adapter real. Default = noop.

## Fase B — Adapter Honcho

1. Crear `internal/agent/infrastructure/honcho/`:
   - `client.go` — cliente HTTP Honcho (crear workspace si no existe,
     buscar/crear peer por agent).
   - `adapter.go` — implementa `MemoryProvider` con `Recall` =
     `peer.context(query, topK=8)` + truncate por budget de tokens, y
     `Remember` = batch `peer.message()` por par.
2. Variables de entorno: `HONCHO_BASE_URL`, `HONCHO_API_KEY`,
   `HONCHO_TIMEOUT`, `HONCHO_TOKEN_BUDGET`.
3. Tests del adapter contra un Honcho mockeado (httptest) — verificar
   keying, top-k, y truncado por tokens.

## Fase C — Integración en el Manager

1. Modificar `Manager.PromptRequest` (`manager.go:438`) para:
   - Llamar `m.memory.Recall(ctx, key, message)` dentro de `callCtx`
     con timeout 2s.
   - Si el resultado no está vacío, prepend vía `runtime.Steer(ctx,
     formattedContext)` antes del `runtime.Prompt` final.
   - Marcar el slot como "steered with memory" para no repetir en
     cada prompt del mismo turno.
2. Suscribirse al evento `agent_end` para hacer `Remember` con los
   pares user/assistant del turno (reusar buffer existente en
   `slot`).

## Fase D — Apagar tools nativos cuando el host enruta

1. Agregar campo `DisableNativeHonchoTools bool` a `StartSpec`.
2. Pasar flag al binario pi (e.g. `--no-honcho-tools` o env var
   equivalente — confirmar con docs de pi).
3. Setearlo a `true` desde el wiring cuando `HONCHO_ENABLED=true`.

## Fase E — Tests de aislamiento y de regresión

1. Tests de aislamiento: dos keys distintas → `Recall` no devuelve
   contexto de la otra key.
2. Tests de regresión: con `noopProvider`, comportamiento idéntico al
   actual (comparar conteo de tokens del system prompt antes/después
   en golden test).
3. Bench: medir tokens del primer turno con/sin Honcho para validar
   el ahorro.

# Links

- Plan general: [doc/agent-teams-plan.md](../../../doc/agent-teams-plan.md) §1,
  §3 Fase 2.
- Punto de inyección: `internal/agent/application/manager.go:438`.
- Steering ya existente: `defaultTurnSteering` en
  `internal/agent/application/manager.go:132` (patrón a reusar).
- Spawn del runtime: `internal/agent/infrastructure/pirpc/process.go`
  (acá se pasa `-e` por cada extensión — otra fuente de inflación de
  tokens, fuera de scope de esta card).
- Card relacionada (keying + tests): se va a crear como parte de la
  Fase E o como card aparte si se quiere trackear por separado.

# Out of Scope

- Multi-agente y workspaces como bounded context (cubierto por cards
  P0/P1 del plan developer-teams).
- Apagar/filtrar las extensiones `.pi/extensions` que también
  inflan tokens — problema distinto, otra card.
- Cambiar el modelo de storage de sesiones del agente.
- Métricas de observabilidad del consumo de tokens (depende del
  provider).

# Riesgos

- **Latencia**: cada prompt paga round-trip a Honcho. Mitigado con
  timeout corto + cache opcional en memoria (no en esta card).
- **Doble consumo** si no se apagan los `honcho_*` tools nativos de
  pi — Fase D lo resuelve.
- **Backcompat**: dev single-agent con `HONCHO_ENABLED` sin set debe
  comportarse idéntico al actual.