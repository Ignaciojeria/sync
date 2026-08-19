# Plan: Plataforma Developer Teams (multi-agente + workspaces)

> **Estado:** borrador inicial. El detalle por tarjeta vive en el backlog
> del repo (`/backlog` en la UI, persistido en `internal/backlog/board/`).
>
> **Principio rector:** separar **definición** (qué es un agente) de
> **runtime** (cómo corre) de **memoria** (qué recuerda). Honcho entra
> solo en la fase de memoria, no toca el resto.

---

## 0. Contexto

Hoy `internal/agent/` es un único runtime acoplado al repo. Para que la
plataforma funcione como "developer teams" necesitamos:

- Múltiples agentes operando en paralelo, cada uno con su **identidad**.
- Múltiples **workspaces** como entidad de primera clase (hoy es el
  CWD implícito del repo).
- Memoria persistente por agente vía Honcho, opt-in, sin acoplar el
  runtime.

No se rompe nada existente: el plan es **aditivo**, todo va detrás de
flags (`AGENT_MULTI`, `HONCHO_ENABLED`) con defaults compatibles.

---

## 1. Capas de responsabilidad

| Capa | Bounded context | Qué define | Estado |
|---|---|---|---|
| Definición | `internal/agentregistry/` (nuevo) | Agente como dato: nombre, prompt, tools, modelo, owner, workspace | P0 |
| Workspaces | `internal/workspace/` (nuevo) | Workspace como dato: dir, miembros, agentes, permisos | P0 |
| Runtime | `internal/agent/` (extender) | Spawn de pi, sandbox, sesiones — ahora multi-instancia | P1 |
| Identidad | `internal/auth/` (extender) | JWT `agent:<id>`, modelo de delegación, audit | P1 |
| Memoria | `internal/agent/infrastructure/honcho/` (nuevo) | Adapter Honcho como `MemoryProvider` | P2 |
| UI | sidenav + páginas `/agents`, `/workspaces` | DaisyUI + HTMX | P3 |

---

## 2. Modelo de datos

```go
// internal/agentregistry/application/agent.go
type Agent struct {
    ID           string    // ulid
    Name         string    // único por workspace
    OwnerEmail   string
    WorkspaceID  string
    SystemPrompt string
    AllowedTools []string  // whitelist
    DefaultModel string    // p.ej. "claude-sonnet-4"
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// internal/workspace/application/workspace.go
type Workspace struct {
    ID        string
    Name      string
    RootDir   string    // sandbox CWD
    Members   []Member  // email + role
    CreatedAt time.Time
}

type Role string
const (
    RoleOwner  Role = "owner"
    RoleEditor Role = "editor"
    RoleMember Role = "member"
)

// Identidad en JWT (internal/shared/claims.go)
type Claims struct {
    Sub      string // humano: email | agente: "agent:<id>"
    ActorSub string // si Sub es agente, quién lo invocó
}
```

### Honcho mapping

| Concepto interno | Concepto Honcho |
|---|---|
| `Workspace` | `workspace` |
| `Agent` | `peer` (peer por agente; humans son otro peer) |
| Mensaje del usuario | mensaje agregado al peer |
| Mensaje del agente | mensaje del peer |
| Resumen de contexto | `deriver` query |

Esto encaja 1:1 con el modelo de Honcho y mantiene memoria aislada por
agente y por workspace.

---

## 3. Fases de implementación

### Fase 0 — Fundacional (P0)
1. `agentregistry`: bounded context nuevo con CRUD + persistencia disco.
2. `workspace`: bounded context nuevo, mismo patrón.

### Fase 1 — Multi-tenant runtime (P1)
3. `agent/runtime`: refactor a multi-instancia por `(agentID, sessionID)`.
4. `auth/identity`: JWT `agent:<id>` + delegación.
5. Flag `AGENT_MULTI` para back-compat.

### Fase 2 — Memoria con Honcho (P2)
6. `honcho` adapter como `MemoryProvider`.
7. Keying aislado por `(workspaceID, agentID)` + tests de aislamiento.

### Fase 3 — UI y permisos (P3)
8. UI `agentregistry` (DaisyUI + HTMX).
9. UI `workspace` con selector de activo.
10. Permisos por rol en workspace.

---

## 4. Reglas de oro

1. **Identidad ≠ Runtime ≠ Definición ≠ Memoria.** Cuatro bounded
   contexts, cuatro carpetas, cero imports cruzados fuera de `shared/`.
2. **Inversión de dependencias.** Las interfaces (`MemoryProvider`,
   `AgentRepository`, `WorkspaceRepository`) viven en `application/`, las
   implementaciones en `infrastructure/`.
3. **Opt-in por flag.** Todo lo nuevo se apaga con env var. El dev
   actual sigue funcionando idéntico con defaults.
4. **Tests primero en domain.** `application/` lleva tests de lógica
   pura antes de cualquier wiring.
5. **Persistencia = mismo patrón.** `disk/<store>.go` con write-through
   + JSON atómico (igual que `backlog` y `agent/sessions`).

---

## 5. Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| Acoplar Honcho al runtime | Interface `MemoryProvider` con no-op por defecto |
| Romper dev single-agent | `AGENT_MULTI=false` por default; rama nueva solo opt-in |
| Memory bleed entre agentes | Tests de aislamiento + keying explícito por peer |
| Permisos mal modelados | Roles en `workspace.Member`, middleware dedicado |
| Sandbox CWD mal resuelto | Reusar `internal/agent/infrastructure/pirpc/sandbox.go` |

---

## 6. Tracking

Las tarjetas concretas viven en el **backlog** del repo (`/backlog`).
La fase 0 arranca con dos cards P0; las demás se mueven a `todo` o
`in_progress` según se prioricen en el tablero.