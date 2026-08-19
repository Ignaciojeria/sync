---
description: 'DEPRECATED — reemplazada por "Despachar cards del backlog a otros agentes". El patrón de supervisor reactivo con marcadores @sup/@plan/@ok/@merge sumaba fricción (7+ interacciones explícitas) sin mejorar UX respecto a hablarle natural a pi. Redirigir a la nueva card.'
priority: P3
source: user
status: done
tags: [deprecated, wontfix, supervisor, agent-ux]
timestamp: "2026-07-18T19:34:34Z"
title: 'DEPRECATED — Supervisor reactivo opt-in para sesiones del agente'
type: backlog/card
---

> ⚠️ Esta card está deprecada. La reemplazó
> *"Despachar cards del backlog a otros agentes vía HTTP"*, que sigue
> el patrón backlog-as-queue con agentes como workers externos.
> Mantener solo como referencia histórica.

# Supervisor reactivo opt-in para sesiones del agente

El usuario hoy opera el agente en 3 turnos manuales por tarea
(abrir sesión → pedir backlog → pedir implementación). Un supervisor
reactivo reduce ese ciclo a 1 turno + confirmaciones puntuales,
**sin tomar decisiones por su cuenta**.

## Contexto

`internal/agent/` ya provee:
- `agentapp.AgentService` (interfaz) + `Manager` (impl).
- `pirpc` con sandbox CWD (`tmp/agent-work/<sessionID>/`) y timeout.
- Session store en disco (`AGENT_SESSION_DIR`).
- UI de chat con go/templ.

Lo que falta es la **capa de control**: decidir cuándo intervenir,
qué proponer y cuándo pedirle confirmación al usuario. Ese es el
alcance de esta card — no rehace el agente, lo envuelve.

## Decisiones de diseño (acordadas en chat)

1. **Opt-in por sesión**. El supervisor arranca inactivo siempre.
2. **Activación manual** por:
   - Marcador `@sup` en cualquier mensaje (atajo de teclado mental).
   - Toggle UI fijo arriba del input: `○ supervisor inactivo` →
     `● supervisor activo`.
   - (Opcional, post-MVP) Idle timeout que sugiere activación vía
     toast — pero nunca activa solo.
3. **Solo propone, nunca ejecuta solo**. Cada acción del catálogo
   pasa por un botón o un `@ack` del usuario antes de ejecutarse.
4. **Catálogo de acciones MVP**:
   - `@backlog "<texto>"` → crea card.
   - `@plan` → pide plan a pi, lo muestra.
   - `@ok` → implementa el último plan aprobado.
   - `@review` → muestra diff + tests corridos.
   - `@merge` → commit / merge (gated por `@review` previo).
   - `@hold` / `@abort X` → pausa o cancela.
5. **Resumen al desactivar**:
   - Estado del backlog tocado en la sesión.
   - Diff acumulado (archivos cambiados, sin commit).
   - Próximo paso sugerido.
6. **Proposer opcional**: un LLM chico detecta verbos de intención
   (*agregá al backlog*, *implementá eso*, *commiteá*) y muestra un
   toast sugiriendo el marcador correspondiente. Solo sugiere.

# Plan

1. **Esqueleto del módulo** — crear `internal/supervisor/` con las 4
   capas vacías (`application/`, `http/`, `infrastructure/`, `ui/`) y
   registrar el wiring en `cmd/api/main.go` con `SUPERVISOR_ENABLED`
   (default `false`). Compila con `go build ./...` aunque el módulo
   sea no-op.

2. **Infra de sesiones** — en `internal/supervisor/infrastructure/store/`:
   struct `SessionState{Active bool, LastPlanID string, DiffHash string}`
   persistido en disco bajo `AGENT_SESSION_DIR`. API: `Get`,
   `SetActive`, `AppendDiff`. Tests unitarios con tmpdir.

3. **Parser de marcadores** — `internal/supervisor/infrastructure/markers/`:
   función `Parse(msg string) []Marker` que reconoce `@sup`,
   `@backlog`, `@plan`, `@ok`, `@review`, `@merge`, `@hold`,
   `@abort`. Tabla-driven tests cubriendo: match exacto, sin match,
   marcadores mal formados, múltiples en un mismo mensaje.

4. **Servicio de aplicación** — `internal/supervisor/application/`:
   `SupervisorService` con `Toggle(ctx, sessionID)`, `HandleMessage`,
   `Summary(ctx, sessionID)`. Consume `agentapp.AgentService` solo vía
   interfaz. Estado: máquina explícita (`inactive` → `idle` →
   `awaiting_plan` → `awaiting_approval` → `running` → `review`).

5. **HTTP endpoints** — `internal/supervisor/http/`:
   - `POST /supervisor/toggle` (HTMX).
   - `POST /supervisor/messages` (intercepta envío).
   - `GET  /supervisor/summary` (al desactivar).
   Registrar con `ioc.Register(...)` siguiendo el patrón del módulo
   agent.

6. **UI** — `internal/supervisor/ui/`:
   - Toggle en `internal/agent/ui/chat.templ` (botón fijo arriba del
     input) que dispara `hx-post="/supervisor/toggle"`.
   - Bloque de resumen cuando el supervisor se desactiva
     (`alert-success` con árbol del estado).
   - Toast opcional para `@ok` sin plan previo (test negativo).

7. **End-to-end** — escenario completo en test de integración:
   abrir sesión → `@sup` → `@backlog` → `@plan` → `@ok` → `@merge`.
   Verifica que el catálogo de marcadores pasa por toda la cadena
   (parser → service → agent → commit) sin acción no autorizada.

8. **Docs** — `doc/supervisor.md` con el catálogo de marcadores,
   ejemplos de uso y limitaciones (out of scope).

## Arquitectura propuesta

```
internal/supervisor/
├── application/         ← SupervisorService: RunTask, Propose, Ack
│                          (consume agentapp.AgentService)
├── http/                ← endpoints /supervisor/*  (toggle, ack, summary)
├── infrastructure/
│   └── markers/         ← parser de @sup / @backlog / @plan / ...
└── ui/                  ← toggle, toasts, barra de confirmaciones
```

Reglas:
- `internal/supervisor/` **no** importa `internal/agent/pirpc`
  directo; pasa por `agentapp.AgentService`.
- Apagado limpio con `SUPERVISOR_ENABLED=false` (igual que
  `AGENT_ENABLED`) — no afecta al resto del wiring.
- Toggle on/off se persiste en la sesión (no es global).

## Flujo de uso esperado

```
tú:    necesito refactorizar el parser de fechas
tú:    @sup backlog "refactor parser de fechas"
sup:   ✓ Backlog #43 creado. ¿Lo planifico?
tú:    dale
sup:   [plan aquí]
tú:    @ok sin el retry exponencial que es overkill
sup:   ✅ tests pasaron · diff: 2 archivos
tú:    @merge
sup:   committed abc123
```

# Acceptance Criteria

- [ ] Existe el módulo `internal/supervisor/` con sus 4 capas
      (`application/`, `http/`, `infrastructure/`, `ui/`).
- [ ] El supervisor arranca inactivo por defecto y respeta el flag
      `SUPERVISOR_ENABLED=false`.
- [ ] El toggle UI cambia el estado de la sesión activa y persiste
      entre mensajes de la misma sesión.
- [ ] El marcador `@sup` alterna el estado (idempotente: si ya está
      activo no hace nada).
- [ ] Los 5 marcadores del catálogo (`@backlog`, `@plan`, `@ok`,
      `@review`, `@merge`) están parseados y testeados.
- [ ] `@hold` y `@abort <razón>` cancelan el paso en curso.
- [ ] Ningún marcador dispara ejecución sin confirmación explícita
      (test: mandar `@ok` sin plan previo no debe implementar nada).
- [ ] Al desactivar se genera un resumen (backlog tocado, diff
      acumulado, próximo paso) y se persiste en la sesión.
- [ ] Tests unitarios del parser (cubren todos los marcadores +
      casos negativos) y de la máquina de estados del supervisor.
- [ ] Endpoint `/supervisor/toggle` (POST) con HTMX para el botón.
- [ ] Documentación mínima en `doc/supervisor.md` con el catálogo de
      marcadores y ejemplos de uso.

# Out of scope (para cards futuras)

- Proposer LLM (detección automática de verbos de intención).
- Idle timeout con sugerencia de activación.
- Multi-step runs automáticos ("ejecutá todo el backlog P1").
- Persistencia cross-sesión del estado del supervisor.
- Métricas / observabilidad de las acciones del supervisor.

# Links

- Depende de `internal/agent/` (estable, listo para envolver).
- Inspirado en el flujo descrito en `doc/backlog-workflow.md`
  (regla 5: "Mover al hablar").
- Patrón de apagado limpio replicado de `AGENT_ENABLED`.

# Examples

## Toggle UI esperado

```
[● Sup activo]  |  escribí tu mensaje...   [Enviar]
```

## Estado al desactivar

```
sup: desactivado. quedaste en:
     ├─ backlog #43 (refactor parser fechas)  [pendiente plan]
     └─ último diff: 2 archivos sin commit
```

## Marcador mal usado (test negativo)

```
tú:    @ok
sup:   no hay plan aprobado para implementar.
       enviá @plan primero.
```

# Citations

[1]: [Internal: AGENTS.md §Módulo agente](../../AGENTS.md)
[2]: [Internal: doc/backlog-workflow.md](../backlog-workflow.md)
