# Plan técnico: merge de previews

## Objetivo

Permitir que una sesión con preview pueda **fusionar su branch de trabajo** sobre el
branch base del repositorio.

La idea es deliberadamente simple:

- cada preview ya vive sobre un workspace/branch aislado
- el merge toma esa branch aislada
- la fusiona al branch base del proyecto

Este documento describe la **fase inicial (v1)**, no el diseño definitivo del
flujo completo de revisión.

---

## Aclaración importante

### Qué se fusiona realmente

Se fusiona la **branch del worktree de la sesión**.

No se fusiona:

- el chat
- el histórico del agente
- los prompts pedidos dentro del agente montado en la preview
- ningún "estado conversacional"

### Regla de producto

La preview puede ser mergeable.

Pero el **agente dentro de la preview** sigue siendo solo para testing/validación.
Sus solicitudes no son la fuente de verdad de la fusión.

---

## 1. Modelo mental del sistema

Hoy el sistema ya tiene gran parte de la base:

- una `Session` del host
- un `WorkspacePath` aislado por sesión
- una `Branch` tipo `agent/<sessionID>`
- una preview HTTP publicada bajo:

```text
/agent/sessions/:id/preview/*
```

Eso alcanza para pensar la operación como:

```text
preview branch -> base branch
```

---

## 2. Decisiones de diseño

## MVP

- fusión local al branch base
- sin push automático
- sin resolución automática de conflictos
- si hay conflicto, se aborta la fusión y se devuelve error claro
- si el repo base está sucio, no se fusiona
- la operación se dispara desde el host
- una preview ya fusionada no debería volver a fusionarse por accidente

## No MVP

- squash configurable
- rebase automático
- push remoto
- resolución asistida de conflictos
- approval workflow complejo
- PRs internas
- merge desde dentro del agente montado en la preview
- merge queues o locks distribuidos
- diff visual complejo

---

## 3. Cambio de modelo de datos

## Archivo a tocar

- `pkg/agent/application/session.go`

## Cambio propuesto

Extender `Session` con metadata suficiente para reproducir el contexto del merge.

```go
type Session struct {
    ID            string     `json:"id"`
    Title         string     `json:"title"`
    CWD           string     `json:"cwd"`
    WorkspacePath string     `json:"workspacePath,omitempty"`
    Branch        string     `json:"branch,omitempty"`
    BaseBranch    string     `json:"baseBranch,omitempty"`
    BaseCommit    string     `json:"baseCommit,omitempty"`
    MergedAt      *time.Time `json:"mergedAt,omitempty"`
    MergedCommit  string     `json:"mergedCommit,omitempty"`
    ...
}
```

## Razón

### `BaseBranch`

Describe contra qué branch nació la preview.

### `BaseCommit`

Describe contra qué snapshot exacto nació la preview.

No es estrictamente imprescindible para v1, pero cuesta poco y deja mejor base
para:

- explicar conflictos
- detectar drift futuro
- evolucionar a estrategias más ricas si luego hace falta

### `MergedAt` / `MergedCommit`

Sirven para:

- marcar que la preview ya fue fusionada
- evitar dobles merges accidentales
- mostrar feedback útil en UI o debugging

---

## 4. Captura del branch y commit base

## Archivo a tocar

- `pkg/agent/infrastructure/worktree/manager.go`

## Cambio propuesto

Cuando se prepara el workspace de una sesión:

1. detectar el branch actual del repo origen
2. detectar el commit actual (`HEAD`) del repo origen
3. guardar ambos como:
   - `BaseBranch`
   - `BaseCommit`
4. seguir creando la branch aislada como hoy

## Resultado esperado

Una sesión mergeable termina con algo así:

```json
{
  "branch": "agent/agent-123",
  "baseBranch": "main",
  "baseCommit": "abc123def456"
}
```

---

## 5. Operación de merge en infraestructura

## Archivo a tocar

- `pkg/agent/infrastructure/worktree/manager.go`

## Método nuevo sugerido

```go
func (m *Manager) MergePreview(ctx context.Context, session agentapp.Session) (MergeResult, error)
```

## Resultado mínimo

```go
type MergeResult struct {
    BaseBranch    string `json:"baseBranch"`
    PreviewBranch string `json:"previewBranch"`
    Commit        string `json:"commit"`
}
```

## Flujo propuesto

1. validar que la sesión tenga:
   - `WorkspacePath`
   - `Branch`
   - `BaseBranch`
2. validar que no esté ya fusionada (`MergedAt` o `MergedCommit`)
3. resolver el repo raíz
4. verificar que el **repo base** esté limpio:
   - `git status --porcelain`
5. checkout del `BaseBranch`
6. intentar merge:
   - `git merge --no-ff --no-edit <previewBranch>`
7. si hay conflicto:
   - `git merge --abort`
   - devolver error claro
8. si sale bien:
   - leer `HEAD`
   - devolver `MergeResult`

## Regla lazy

Nada de estrategia configurable en la primera versión.

Primero:

- merge normal
- abort si falla

---

## 6. Definición precisa de “repo limpio” 

Este punto tiene que quedar explícito para evitar ambigüedad.

## Regla

El repo que debe estar limpio es el **repo base donde se ejecutará el merge**.

No significa que el workspace preview deba estar "sin cambios"; justamente la
preview existe para contener esos cambios.

## Qué validar

Antes del merge:

```text
git status --porcelain
```

en el repo base.

Si no está vacío:

- no se fusiona
- devolver error claro

---

## 7. Errores esperables

## Casos que deben fallar limpio

### Sesión no mergeable

Falta alguno de estos datos:

- `WorkspacePath`
- `Branch`
- `BaseBranch`

### Preview ya fusionada

Si la sesión ya tiene:

- `MergedAt`
- o `MergedCommit`

entonces no se vuelve a fusionar automáticamente.

### Repo base sucio

Si el repo base tiene cambios locales pendientes:

- no se fusiona
- devolver error `409` o equivalente

### Conflicto

Si la fusión genera conflicto:

- abortar merge
- devolver error claro
- no dejar el repo a mitad de camino

### Branch inexistente

Si la branch preview ya no existe o el workspace quedó inconsistente:

- devolver error explícito

---

## 8. Exponer la operación en application

## Archivo a tocar

- `pkg/agent/application/manager.go`

## Cambio propuesto

Agregar al `AgentService` una operación con nombre explícito.

```go
MergePreview(ctx context.Context, id string) (MergeResult, error)
```

## Wiring sugerido

Seguir el patrón ya usado para prepare/destroy:

```go
WithSessionMerger(fn func(context.Context, Session) (MergeResult, error)) *Manager
```

## Implementación del manager

`Manager.MergePreview(...)` debería:

1. buscar la sesión
2. validar que exista
3. validar que sea mergeable
4. delegar al merger inyectado
5. si sale bien, persistir:
   - `MergedAt`
   - `MergedCommit`
6. propagar el resultado o el error

## Razón del naming

`MergePreview` expresa mejor la intención que un `Merge` genérico.
No estamos exponiendo una API Git arbitraria, sino una acción concreta del
producto.

---

## 9. Wiring en `cmd/api`

## Archivo a tocar

- `cmd/api/main.go`

## Cambio propuesto

En el setup del agente:

```go
manager = manager.
  WithSessionPreparer(...).
  WithSessionDestroyer(...).
  WithSessionMerger(workspaceManager.MergePreview)
```

## Resultado

El host ya puede pedir:

```text
session id -> merge preview branch -> base branch
```

sin acoplar HTTP a Git.

---

## 10. Endpoint HTTP

## Archivo nuevo sugerido

- `pkg/agent/http/merge.go`

## Ruta propuesta

```http
POST /agent/sessions/{id}/merge
```

## Respuesta exitosa

```json
{
  "ok": true,
  "baseBranch": "main",
  "previewBranch": "agent/agent-123",
  "commit": "abc123def456"
}
```

## Status sugeridos

- `200` fusión exitosa
- `404` sesión no encontrada
- `400` sesión no mergeable o ya fusionada
- `409` conflicto o repo base sucio
- `500` error inesperado

---

## 11. Restricciones de producto

## No fusionar desde dentro de la preview

El botón/acción de merge no debería vivir dentro del agente montado en la preview.

Razón:

- ese agente es solo para testing
- ya mostramos explícitamente que sus solicitudes no cambian la fusión final

## Sí fusionar desde el host

La acción real de merge debe existir en la sesión preview del host.

O sea:

- el host administra
- la preview valida

---

## 12. UI mínima

## Archivos candidatos

- `pkg/agent/ui/page.templ`
- JS ya existente del agent page

## MVP de UX

Mostrar un botón:

- `Merge preview`

Solo cuando la sesión sea mergeable.

## Acción del botón

1. confirmación simple
2. `POST /agent/sessions/{id}/merge`
3. mostrar:
   - success con commit resultante
   - o error claro si hubo conflicto/repo sucio

## Mensaje recomendado ante conflicto

```text
La preview ya no puede fusionarse automáticamente. Actualiza la preview o resuelve los conflictos manualmente.
```

Eso evita que el usuario piense que el sistema falló, cuando en realidad Git
está haciendo lo correcto.

## Qué no hacer todavía

- wizard
- diff visual complejo
- panel de conflictos
- push remoto

---

## 13. Estado después de una fusión exitosa

Este punto conviene decidirlo explícitamente desde v1.

## Recomendación

### No destruir automáticamente la preview

Después de un merge exitoso:

- no borrar el workspace automáticamente
- no matar la preview automáticamente

### Sí marcar la sesión como fusionada

Persistir en la sesión:

- `MergedAt`
- `MergedCommit`

### Sí bloquear dobles merges accidentales

Si alguien intenta volver a fusionar la misma preview:

- devolver error claro
- o deshabilitar el botón en UI

## Razón

Esto mantiene la primera versión simple y reversible.
Destruir automáticamente la preview puede ser útil más adelante, pero hoy añade
más decisiones operativas de las necesarias.

---

## 14. Tests

## Infraestructura Git

### Archivo

- `pkg/agent/infrastructure/worktree/manager_test.go`

### Casos mínimos

1. merge exitoso
2. conflicto
3. repo base sucio
4. sesión sin branch/baseBranch
5. sesión ya fusionada

## Application

### Archivo

- `pkg/agent/application/manager_test.go`

### Casos mínimos

1. `Manager.MergePreview` delega bien al merger
2. persiste `MergedAt` y `MergedCommit` tras éxito
3. propaga conflictos y errores

## HTTP

### Archivo nuevo sugerido

- `pkg/agent/http/merge_test.go`

### Casos mínimos

1. `POST /merge` exitoso
2. sesión inexistente
3. conflicto devuelve `409`
4. sesión no mergeable devuelve `400`
5. sesión ya fusionada devuelve `400`

---

## 15. Orden de implementación recomendado

1. agregar `BaseBranch`, `BaseCommit`, `MergedAt`, `MergedCommit` a `Session`
2. persistir `BaseBranch` y `BaseCommit` al preparar worktree
3. implementar `worktree.Manager.MergePreview`
4. exponer `MergePreview` en `AgentService`
5. wiring en `cmd/api`
6. endpoint HTTP
7. botón UI mínimo
8. tests

---

## 16. Riesgos y mitigaciones

## Riesgo: repo base quedó sucio

Mitigación:

- bloquear fusión si `git status --porcelain` no está vacío

## Riesgo: conflictos

Mitigación:

- `git merge --abort`
- devolver error claro

## Riesgo: branch base cambió desde que nació la preview

Mitigación:

- persistir `BaseBranch`
- idealmente también `BaseCommit`

## Riesgo: doble merge accidental

Mitigación:

- persistir `MergedAt` / `MergedCommit`
- bloquear un segundo merge

## Riesgo: confusión entre preview-agent y merge real

Mitigación:

- mantener mensaje actual en preview:
  - ese agente solo prueba la preview actual
  - no cambia la fusión final

---

## 17. Definición de done

Está done cuando:

1. una sesión preview conoce su `BaseBranch`
2. idealmente también conoce su `BaseCommit`
3. existe una operación `MergePreview(ctx, id)` en `AgentService`
4. el host expone `POST /agent/sessions/{id}/merge`
5. la fusión une `preview branch -> base branch`
6. si hay conflicto, aborta limpio
7. si el repo base está sucio, no fusiona
8. una preview ya fusionada no se puede fusionar otra vez por accidente
9. la UI del host puede disparar la fusión
10. el agente montado en preview sigue siendo solo testing

---

## 18. Recomendación final

Implementar v1 así:

- `BaseBranch`
- `BaseCommit` si sale barato
- fusión local
- sin push
- sin conflictos automáticos
- endpoint HTTP simple
- botón UI mínimo
- marcar sesión como fusionada

Es el camino más corto que entrega valor real sin abrir una caja de Pandora de
flujos Git innecesarios.
