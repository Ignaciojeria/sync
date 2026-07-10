# Session Runtime Plan

## Resumen

Este documento propone una implementación v1 para evitar que el chat del agente rompa su propia sesión cuando modifica código observado por `air`.

La decisión central es esta:

> **Sync no administra worktrees; administra sesiones.**
> Cada sesión tiene un entorno aislado de trabajo.
> Hoy ese entorno se implementa con `git worktree`.

## Problema

Hoy el agente corre embebido en `cmd/api` y expone una interfaz de chat. Si el agente modifica archivos del mismo working tree que observa `air`, el proceso se recompila y la comunicación del chat se corta.

El problema real no es la branch; es el **working tree compartido**.

## Objetivo del MVP

Permitir que cada sesión del chat trabaje sobre una copia aislada del código, sin afectar el checkout principal ni interrumpir el proceso host.

## Principios

1. **La sesión es la unidad del sistema.**
2. **El agente no modifica el checkout principal.**
3. **El aislamiento es físico, no solo lógico.**
4. **El agente no ejecuta Git directamente.**
5. **La promoción de cambios es explícita y controlada.**
6. **No diseñar para múltiples backends ahora, pero no cerrarse a eso.**

## Modelo mental

Una sesión es un entorno efímero o persistente donde un agente puede trabajar de forma aislada.

Componentes de una sesión en v1:

- conversación
- estado persistente básico
- workspace aislado
- branch efímera
- preview por sesión
- mecanismo de promoción de cambios

El `git worktree` deja de ser el concepto principal y pasa a ser un recurso interno de la sesión.

## Decisiones de arquitectura

### 1. Una sesión usa un workspace aislado

Cada sesión debe trabajar sobre un directorio distinto del checkout principal.

Ejemplo:

```txt
repo-main/                 <- host estable, observado por air
session-workspaces/
  session-123/
  session-456/
```

### 2. No hay worktrees anidados

Si una sesión A crea una sesión B, B se crea como hermana, no como hija física.

Correcto:

```txt
session-workspaces/
  session-a/
  session-b/
```

Incorrecto:

```txt
session-workspaces/
  session-a/
    session-b/
```

La jerarquía lógica, si se necesita después, vive en metadatos, no en el filesystem.

### 3. El runtime orquesta; los managers ejecutan

Para v1 no hace falta un framework de interfaces ni múltiples providers.

Estructura sugerida:

- `SessionService`: orquesta creación, ciclo de vida y promoción
- `WorktreeManager`: sabe crear/destruir branches y worktrees
- `AgentManager`: sabe iniciar y detener agentes por sesión
- `PreviewManager`: sabe levantar o resolver preview por sesión

### 4. Git es implementación, no contrato público

El agente no debería conocer `git worktree add`, `git branch` ni rutas internas.

El contrato público debe hablar de sesiones.

## Modelo mínimo de datos

```go
type Session struct {
    ID            string
    WorkspacePath string
    Branch        string
    PreviewURL    string
    Status        string
}
```

Notas:

- `PreviewURL` puede existir desde el día uno aunque inicialmente esté vacío.
- No agregar todavía campos como `Budget`, `Lineage`, `Policies` o `History` si no viven realmente en el runtime.

## Flujo del MVP

### Crear sesión

`CreateSession()` debe:

1. generar `SessionID`
2. crear branch efímera
3. crear worktree aislado
4. apuntar el agente a ese path
5. levantar preview mínimo por sesión
6. persistir metadatos básicos de la sesión
7. devolver `SessionID` y `PreviewURL`

### Operar sesión

Durante la sesión:

- el chat conversa con el agente
- las tools trabajan dentro del `WorkspacePath`
- el checkout principal no se toca
- `air` no observa cambios del sandbox de la sesión

### Aprobar e integrar

Cuando el usuario aprueba el resultado:

- integración manual controlada
- opción preferida en v1: merge o cherry-pick controlado
- opción alternativa/fallback: exportar patch

Importante:

- **Promote** no es lo mismo que **Merge**
- en v1 ambos pueden estar muy cerca, pero conviene mantener esa distinción conceptual

### Cerrar sesión

`DestroySession()` debe:

1. detener agente
2. bajar preview
3. liberar recursos asociados
4. eliminar worktree
5. marcar sesión como cerrada o archivada

## MVP funcional

Para que el producto sea usable, el MVP debería incluir:

1. `SessionService`
2. `WorktreeManager`
3. branch efímera por sesión
4. worktree aislado por sesión
5. `AgentManager`
6. preview mínimo por sesión
7. `DestroySession()`
8. integración manual controlada

## No entra en v1

Queda explícitamente fuera:

- providers genéricos
- múltiples backends de sandbox
- contenedores o VMs
- lineage complejo padre/hijo
- presupuestos sofisticados
- políticas avanzadas de review/promoción
- telemetría extensa
- automatización completa de merge

## Roadmap sugerido

### Sprint 1

- `SessionService`
- `WorktreeManager`
- branch efímera por sesión
- worktree aislado por sesión
- persistencia mínima de sesiones
- `DestroySession()`

### Sprint 2

- `AgentManager` por sesión
- preview mínimo por sesión
- URL temporal estable
- validación de que el agente opera dentro del sandbox

### Sprint 3

- flujo de aprobación
- merge o cherry-pick controlado
- fallback `ExportPatch()` si conviene
- archivado de sesiones

### Sprint 4

- mejorar persistencia de conversación/estado
- historial de sesiones
- reanudación de sesión
- cleanup de huérfanos

## Riesgos principales

1. **Seguir usando el working tree principal**
   - mantiene el problema original

2. **Meter demasiada abstracción temprano**
   - retrasa el MVP sin aportar valor inmediato

3. **Modelar jerarquía física de sesiones**
   - complica rutas, cleanup y debugging

4. **No definir bien la promoción de cambios**
   - deja la experiencia truncada aunque el sandbox funcione

5. **No persistir suficiente estado de sesión**
   - impide retomar trabajo real después

## Criterios de éxito

Se considera exitoso el MVP si:

1. un usuario puede abrir una sesión de chat
2. el agente modifica código sin disparar recompilación del host
3. la sesión tiene un preview propio
4. el usuario puede aprobar e integrar cambios
5. la sesión puede cerrarse sin dejar recursos huérfanos

## Decisión final

La línea a seguir es:

- construir un **Session Runtime**
- tratar el `git worktree` como detalle interno
- aislar físicamente el código por sesión
- mantener el checkout principal estable
- priorizar una experiencia usable antes que una arquitectura extensible prematuramente

## Próximos pasos

1. decidir ubicación física de los workspaces de sesión
2. definir `SessionService` y `Session` mínimos
3. implementar creación y destrucción de sesiones
4. redirigir el agente para que use siempre el `WorkspacePath` de su sesión
5. agregar preview mínimo por sesión
6. definir flujo de aprobación e integración manual
