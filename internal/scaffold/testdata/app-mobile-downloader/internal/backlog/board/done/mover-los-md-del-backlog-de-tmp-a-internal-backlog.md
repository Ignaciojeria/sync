---
description: Reubicar el bundle OKF del backlog (tarjetas, AGENTS.md, index.md) como datos persistentes del módulo internal/backlog, en línea con la convención de bounded contexts del proyecto.
priority: P2
source: user
status: done
timestamp: "2026-07-18T18:00:58Z"
title: Mover los .md del backlog de tmp a internal/backlog
type: backlog/card
---
# Contexto

El módulo `internal/backlog/` (código Go: `spec.go`, `application/`,
`http/`, `infrastructure/`, `ui/`, `SPEC.md`) convive con su **bundle
de datos** (`tmp/backlog/`: `AGENTS.md`, `index.md`, las cuatro
columnas `backlog/`, `todo/`, `in_progress/`, `done/`). Ese bundle
es estado **persistente del módulo**, no algo runtime/volátil: las
tarjetas viven vía `BACKLOG_DIR` y son datos de primer nivel del
dominio. Tenerlo en `tmp/` rompe la convención de bounded contexts
que el resto del repo sí respeta (auth, agent, editor, home,
quality, scheduler, etc. tienen todo su material dentro de
`internal/<modulo>/`).

# Decisiones a tomar antes de empezar

- [ ] **Subpath dentro del módulo.** Opciones:
  - `internal/backlog/board/` (subdir dedicado, evita mezclar data
    con código; "board" alude al tablero kanban).
  - `internal/backlog/` directo (sin subdir). Más corto pero mezcla
    `spec.go` con `.md` del bundle en el listado del módulo.
  - `internal/backlog/data/` (similar a `board/`, nombre más genérico).
  - Recomendación: **`internal/backlog/board/`**.
- [ ] **Ubicación del `AGENTS.md` del bundle.** Es el system prompt
  del agente que opera sobre el bundle. Opciones:
  - Quedarse en `<BOARD_DIR>/AGENTS.md` (ley por el SPEC §3: lo lee
    pi runtime al arrancar en ese directorio).
  - Moverse a `.agents/skills/...` y duplicar / linkear desde el
    bundle.
  - Recomendación: **quedarse en `<BOARD_DIR>/AGENTS.md`** para
    preservar el flujo actual sin tocar el agent runtime.
- [ ] **Concurrencia / atomicidad.** El `mv` debe ser `git mv` para
  preservar historia. El commit debe contener:
  - El move de todos los `.md` del bundle (un commit en sí mismo
    es razonable).
  - Las actualizaciones de paths (default `BACKLOG_DIR`, doc,
    scripts) pueden ir en commits separados o en el mismo PR si
    están bien delimitados.

# Plan de implementación

## Fase 0 — Auditoría de lectores del path

Antes de mover nada, mapear quién lee/escribe `tmp/backlog/`:

1. `grep -rn "tmp/backlog" .` (excluir `node_modules`, `.agents/`).
2. Resultado conocido hasta ahora (verificar exhaustivo):
   - `doc/backlog-workflow.md` — doc de uso del CLI.
   - `cmd/api/main.go` (l. 217–219) — fallback de `BACKLOG_DIR`.
   - `tmp/backlog-cli/main.go` (l. 88) — fallback CLI.
   - `tmp/backlog/AGENTS.md` (l. 10) — system prompt del agente.
   - Cualquier script en `scripts/` que lo asuma.
3. Documentar cada caso en un bullet de "lectores a actualizar".

## Fase 1 — Crear el destino del bundle

1. Crear `internal/backlog/board/` con las cuatro columnas y `.keep`:
   ```sh
   mkdir -p internal/backlog/board/{backlog,todo,in_progress,done}
   touch internal/backlog/board/{backlog,todo,in_progress,done}/.keep
   ```
2. Confirmar que el layout cumple con `internal/backlog/SPEC.md §3`.

## Fase 2 — Mover el bundle (git-aware)

Usar `git mv` para preservar historia. Atajo para todo el árbol:

```sh
git mv tmp/backlog/AGENTS.md     internal/backlog/board/AGENTS.md
git mv tmp/backlog/index.md      internal/backlog/board/index.md
git mv tmp/backlog/backlog       internal/backlog/board/backlog
git mv tmp/backlog/todo          internal/backlog/board/todo
git mv tmp/backlog/in_progress   internal/backlog/board/in_progress
git mv tmp/backlog/done          internal/backlog/board/done
```

> **Nota**: si git se queja por subdirs no vacíos, mover archivo por
> archivo (`git mv src/* internal/backlog/board/<col>/`).

## Fase 3 — Actualizar lectores

1. **Default de `BACKLOG_DIR`** en `cmd/api/main.go` (l. 219):
   - Cambiar `if dir == "" { dir = "tmp/backlog" }` → `dir =
     "internal/backlog/board"`.
2. **Default de `BACKLOG_DIR`** en `tmp/backlog-cli/main.go` (l. 88):
   - Idem.
3. **`doc/backlog-workflow.md`**:
   - Reemplazar todas las apariciones literales de
     `BACKLOG_DIR=tmp/backlog` por `BACKLOG_DIR=internal/backlog/board`.
   - Aclarar que el default ahora apunta adentro del módulo.
4. **`internal/backlog/board/AGENTS.md`** (l. 10):
   - Cambiar la mención "típicamente tmp/backlog" por la nueva ruta.
5. **`scripts/`** (si aplica):
   - Buscar referencias con `grep -rn "tmp/backlog" scripts/` y
     actualizar.
6. **`.gitignore`**:
   - Verificar que `tmp/` siga siendo ignorada, y que
     `internal/backlog/board/` **NO** esté ignorada (debe commitearse).

## Fase 4 — Verificación

1. **Compilación y tests**:
   ```sh
   go build ./...
   go test ./...
   ```
   (Atención: `internal/backlog/application` y `internal/backlog/http`
   no deberían romperse porque leen `BACKLOG_DIR` por env var — solo
   cambiamos el default.)
2. **CLI contra el nuevo path**:
   ```sh
   BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli list
   ```
   Debe listar las mismas tarjetas que estaban en `tmp/backlog/`
   (kanban ordenado por prioridad, mismas P0/P1/...).
3. **Server arranca sin `BACKLOG_DIR`**:
   ```sh
   go run ./cmd/api
   ```
   Debe tomar el nuevo default y servir el bundle sin 404s en las
   rutas `/backlog/...`.
4. **UI smoke test**: navegar a `/backlog` (o la ruta equivalente)
   y verificar que las tarjetas existentes se renderizan.
5. **Concurrencia / flock**: ejecutar una mutación (`create` o
   `move`) y otra lectura (`list`) en paralelo para descartar
   regresiones en `internal/backlog/infrastructure/fs`.

## Fase 5 — Cleanup

1. Si `tmp/backlog/` quedó vacío tras el move, eliminar el dir:
   ```sh
   rmdir tmp/backlog  # sólo si está vacío
   ```
2. Si quedó algo (ej. un binario de `go run ./tmp/backlog-cli` cache),
   hacer `git clean -fdx tmp/` con cuidado.
3. Commit final de cleanup en un commit aparte y mensaje claro:
   `chore(backlog): move bundle from tmp/backlog to internal/backlog/board`.

# Acceptance Criteria

- [ ] `BACKLOG_DIR` default en `cmd/api/main.go` apunta a
      `internal/backlog/board`.
- [ ] `BACKLOG_DIR` default en `tmp/backlog-cli/main.go` apunta a
      `internal/backlog/board`.
- [ ] `doc/backlog-workflow.md` no contiene referencias literales a
      `tmp/backlog`; todas apuntan al nuevo path.
- [ ] `internal/backlog/board/AGENTS.md` actualizado con la nueva ruta.
- [ ] `internal/backlog/board/{index.md,AGENTS.md}` y los cuatro
      subdirs de columnas están commiteados (no ignorados por
      `.gitignore`).
- [ ] `internal/backlog/board/<col>/*.md` contiene las **mismas**
      tarjetas que tenía `tmp/backlog/<col>/*.md` (verificable con
      `git log --follow` o diff).
- [ ] `go build ./...` y `go test ./...` pasan.
- [ ] El server arranca con `BACKLOG_DIR` no seteado y sirve el
      bundle correctamente.
- [ ] No quedan referencias rotas a `tmp/backlog/` en el repo
      (verificación: `grep -rn "tmp/backlog" --include="*.go"
      --include="*.sh" --include="*.md" .` solo matchea este tarjeta
      o referencias intencionales / históricas).
- [ ] Si `tmp/backlog/` quedó vacío y no se usa más, fue eliminado.

# Links

- Spec del módulo: `internal/backlog/SPEC.md` (§3 Layout en disco,
  §6 Naming, §8 Concurrencia).
- Card semilla relacionada: [Plan developer-teams](../../todo/...md)
  (la que `tmp/backlog-cli seed` siembra — verificar que sigue
  funcionando con el nuevo path).
- Card relacionada: [Mejorar el detalle del backlog agregando
  sección de plan](../done/mejorar-el-detalle-del-backlog-agregando-seccion-de-plan.md)
  (la completamos con esta card).

# Citations

- [1] `internal/backlog/SPEC.md` §3 — Layout en disco del bundle.
- [2] `internal/backlog/SPEC.md` §6 — Algoritmo de naming de
  archivos.
- [3] `okf_version: "0.1"` declarado en `tmp/backlog/index.md` —
  preservar al migrar.

