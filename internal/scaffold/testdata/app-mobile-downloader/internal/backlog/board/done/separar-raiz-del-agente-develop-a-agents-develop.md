---
description: Mover .pi y AGENTS.md del agente a agents/develop/ para sentar las bases de múltiples agentes. Adelgazar el AGENTS.md raíz a reglas globales del proyecto.
priority: P1
source: user
status: done
tags:
    - agent
    - infra
    - refactor
    - multi-agent
timestamp: "2026-07-20T18:14:07Z"
title: Separar raíz del agente develop a agents/develop con .pi y AGENTS.md propios
type: backlog/card
---
# Separar raíz del agente develop a agents/develop con .pi y AGENTS.md propios

Hoy el agente arranca con el `.pi/` y el `AGENTS.md` en la **raíz del
proyecto**. Eso mezcla dos cosas distintas:

1. Reglas del proyecto que aplican a todo humano o IA (estilo, commits,
   skills, estructura, auth, design system, etc.).
2. Cómo opera **este agente** dentro del proyecto (runtime, capas,
   sandbox, opt-out).

Esas dos cosas van a divergir cuando existan más agentes (`reviewer`,
`docs`, etc.). Este card sienta las bases para multi-agente creando
`agents/develop/` como el primer "workspace" aislado de un agente.

## Contexto

- `internal/agent/` corre pi embebido en `cmd/api` y usa el sandbox
  definido en `internal/agent/infrastructure/pirpc/sandbox.go`.
- `resolveCWD` redirige CWD vacío/`.` a `tmp/agent-work/<sessionID>/`
  y siembra `.pi` desde la raíz del repo (`seedPiConfig`).
- El `AGENTS.md` raíz lo consume este agente (pi) como system prompt
  del proyecto.
- Hoy no existe ninguna carpeta `agents/` en el repo.

## Decisiones de diseño

1. **El root `AGENTS.md` se conserva pero se adelgaza.** Quedan las
   reglas globales del proyecto (estructura, convenciones frontend,
   design system, skills, módulos, flujo de tests, auth). Se mueven al
   `agents/develop/AGENTS.md` lo específico del runtime del agente:
   capas del módulo `internal/agent/`, sandbox CWD, opt-out,
   `AGENT_ENABLED`.
2. **El `.pi/` se mueve íntegro a `agents/develop/.pi/`.** Sin
   transformaciones. El sandbox se siembra desde la nueva ubicación.
3. **El cambio es transparente para sesiones nuevas, pero no migra
   sesiones existentes.** Las sesiones viejas en `tmp/agent-work/`
   siguen funcionando con el `.pi` viejo que quedó copiado dentro del
   sandbox. No hay script de migración en este card.
4. **Solo se crea `agents/develop/` por ahora.** El registry de
   agentes (lista en código de `internal/agent/application/`) se
   introduce con un solo entry `"develop"`. Agregar más agentes es
   trabajo de cards futuros.
5. **El `AGENT_DEFAULT_ID` env var existe pero default es `"develop"`.**
   Mantiene la API actual: `AgentService.CreateSession` sin
   `AgentID` sigue funcionando.

## Cambios concretos

1. **Mover `.pi/`** de la raíz a `agents/develop/.pi/` (commit aparte
   para que `git` detecte el rename correctamente).
2. **Crear `agents/develop/AGENTS.md`** con el contenido específico
   del agente develop, extraído del `AGENTS.md` raíz actual:
   - Capas de `internal/agent/`.
   - Reglas del sandbox CWD y `tmp/agent-work/`.
   - Opt-out con `AGENT_ENABLED=false`.
   - Nota histórica del runtime actual (cmd/api único).
3. **Adelgazar `AGENTS.md` raíz** quitando las secciones que pasaron
   al agente. Lo que queda: estructura del proyecto, capas, frontend,
   design system, skills, tests, auth.
4. **`AgentService.CreateSession`** (`internal/agent/application/`):
   agregar campo `AgentID string` al input. Default `"develop"` cuando
   viene vacío.
5. **`pirpc.resolveCWD`** (`internal/agent/infrastructure/pirpc/sandbox.go`):
   - Recibir `agentID`.
   - Sembrar `.pi` desde `agents/<agentID>/.pi/` en vez de la raíz.
   - Copiar `agents/<agentID>/AGENTS.md` al sandbox (igual que se
     copia `.pi` hoy).
6. **Registry de agentes** — nuevo archivo chico en
   `internal/agent/application/registry.go`:
   ```go
   var DefaultAgents = []AgentDescriptor{
       {ID: "develop", Label: "Develop", Default: true},
   }
   ```
   Consumido por el `Manager` para resolver el `AgentID` cuando el
   caller no lo especifica.
7. **Tests:**
   - Unitario de `resolveCWD` con agentID="develop" verifica que el
     `.pi` se copia desde la nueva ubicación y que `AGENTS.md` queda
     dentro del sandbox.
   - Unitario de `CreateSession` con `AgentID=""` resuelve a
     `"develop"`.
   - Smoke: crear sesión via HTTP, verificar que el sandbox tiene
     `AGENTS.md` con el contenido del agente develop.
8. **Documentación:**
   - `doc/agent-runtime.md` agrega una nota apuntando a
     `agents/develop/AGENTS.md` como fuente de reglas específicas.
   - No se reescribe el runtime doc completo en este card.

## Lo que NO hace este card

- **No introduce el sidenav con grupo "Agentes".** Eso es un card
  aparte (cambia `NavigationContext`, `sidenav.templ` y routing).
- **No cambia el routing** `/agent/*` a `/agent/<id>/*`. Se mantiene
  el routing actual con `agentID` como dato de sesión, no como
  segmento de URL. La migración de URL queda para el card del sidenav.
- **No crea agentes adicionales** (`reviewer`, `docs`, etc.). Solo
  deja la infra lista.
- **No toca el tema** (sigue global, cookie `design-theme`).
- **No introduce acento por agente** (`data-agent`, `--pi-accent`).
- **No migra sesiones existentes** en `tmp/agent-work/`. Si el
  humano lo necesita, es un card aparte.
- **No cambia `cmd/api`** ni el wiring de encendido (`AGENT_ENABLED`).

## Acceptance Criteria

- [ ] Existe `agents/develop/.pi/` con todo el contenido del antiguo
      `.pi/` de la raíz (verificable con `git log --follow` o diff
      rename detection).
- [ ] Existe `agents/develop/AGENTS.md` con las secciones específicas
      del agente develop (capas, sandbox, opt-out, runtime actual).
- [ ] El `AGENTS.md` raíz ya **no** contiene esas secciones
      específicas del agente; conserva las reglas globales.
- [ ] `internal/agent/application/` tiene un `registry.go` con al
      menos el descriptor `develop`.
- [ ] `AgentService.CreateSession` acepta `AgentID`. Cuando viene
      vacío, resuelve a `"develop"`. Test unitario lo cubre.
- [ ] `pirpc.resolveCWD(agentID, sessionID)` siembra `.pi` y
      `AGENTS.md` desde `agents/<agentID>/`. Test unitario lo cubre.
- [ ] Una sesión nueva creada via HTTP tiene dentro de su sandbox
      (`tmp/agent-work/<sessionID>/`) tanto `.pi/` como `AGENTS.md`
      con el contenido de `agents/develop/`.
- [ ] El `AGENTS.md` raíz **no** se siembra dentro del sandbox (ya
      no es fuente de reglas del agente; las globales quedan solo
      para el LLM del humano que abre el repo).
- [ ] `go build ./...` y `go test ./...` pasan en verde.
- [ ] `doc/agent-runtime.md` referencia `agents/develop/AGENTS.md`.

## Dependencias

- `internal/agent/` (existente, extender `CreateSession` y
  `resolveCWD`).
- Ningún cambio en `cmd/api`, `internal/auth/`, `internal/ui/`,
  ni en el sidenav.

## Links

- Hilo de conversación en sesión: decisiones de diseño sobre multi-
  agente y aislamiento de workspaces.
- Inspirado en el patrón "opt-out limpio" ya usado con
  `AGENT_ENABLED` (referenciado en el AGENTS.md raíz).
- Sigue a: este card habilita los futuros
  *Sidenav con grupo Agentes* y *Acento visual por agente*, pero no
  los implementa.

## Examples

### Estructura del repo después del cambio

```
repo/
├── AGENTS.md                      ← reglas globales (adelgazado)
├── agents/                        ← nuevo
│   └── develop/
│       ├── AGENTS.md              ← reglas específicas del agente
│       └── .pi/                   ← movido desde la raíz
└── ...
```

### Antes / después de `resolveCWD` (pseudocódigo)

```go
// antes
func resolveCWD(specCWD, sessionID string) (string, error) {
    // siembra .pi desde "./.pi" (raíz)
}

// después
func resolveCWD(specCWD, sessionID, agentID string) (string, error) {
    if agentID == "" {
        agentID = "develop"
    }
    src := filepath.Join("agents", agentID)
    // siembra .pi desde agents/<id>/.pi
    // copia agents/<id>/AGENTS.md al sandbox
}
```
