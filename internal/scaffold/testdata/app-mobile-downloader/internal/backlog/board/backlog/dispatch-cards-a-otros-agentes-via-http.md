---
description: 'Permitir que agentes externos (otros procesos pi, workers, otros VMs) saquen cards del backlog y las ejecuten, dejando el resultado de vuelta en la card. El backlog pasa de ser "lista de pendientes humana" a "queue de work orders". Este chat (pi) sigue siendo el orquestador que define tareas; la ejecución se delega. Sin scheduler automático en MVP: protocolo HTTP simple entre agentes.'
priority: P1
source: user
status: backlog
tags: [dispatch, multi-agent, backlog, worker]
timestamp: "2026-07-18T19:55:00Z"
title: 'Despachar cards del backlog a otros agentes vía HTTP'
type: backlog/card
---

# Despachar cards del backlog a otros agentes vía HTTP

Reemplaza la card deprecada
*"Supervisor reactivo opt-in para sesiones del agente"*. Estrategia:
el backlog deja de ser solo una lista humana y pasa a ser una
**queue de work orders** que agentes externos pueden tomar y
devolver con resultado.

## Contexto

Hoy el flujo es 100% manual:
- `internal/backlog/` persiste cards como markdown en disco.
- `internal/scheduler/` ya tiene `JobRunner` con `DistributedLock`
  y `InternalHTTPClient`, pero está pensado para jobs cron (no para
  consumir una queue).
- `internal/agent/` corre pi embebido en `cmd/api` y es el único
  ejecutor.
- No hay forma de que otro agente tome una card.

Falta el **puente** entre la queue (backlog) y los workers (otros
agentes). Eso es lo que define este card — no rehace el backlog ni
el agent, los conecta.

## Decisiones de diseño

1. **El chat (pi) sigue siendo orquestador.** Define tareas y las
   crea como cards. No ejecuta cards (salvo la propia sesión).
2. **Los agentes externos son workers.** Corren en otros procesos o
   VMs. Cada uno tiene identidad (`agentID`) y permisos.
3. **El despacho es por HTTP.** Sin scheduler automático en MVP. Un
   worker hace `POST /backlog/claim` periódicamente o por evento.
4. **Lease con TTL.** Toda card `claimed` tiene un lease. Si vence
   sin heartbeat, vuelve a `backlog` automáticamente.
5. **Identidad del agente via JWT.** Sub-claim `kind=agent` o un
   secret separado. Distinto de usuarios humanos (`allowedAppEmails`).
6. **Feedback loop obligatorio.** El worker actualiza la card al
   terminar: `status=done` (o `blocked`), diff, nota, link al
   resultado. Sin esto la queue no sirve.

## Modelo de estados propuesto (extensión del actual)

Actual: `backlog → todo → in_progress → done`.

Nuevo:

| Estado     | Significado                                              |
|------------|----------------------------------------------------------|
| `backlog`  | Pendiente, nadie la tomó                                 |
| `claimed`  | Asignada a un agente, esperando que arranque             |
| `running`  | Agente trabajando activamente                            |
| `review`   | Terminó ejecución, esperando revisión humana            |
| `done`     | Cerrada OK                                               |
| `blocked`  | Worker reportó error, requiere intervención              |

`claimed` y `running` se diferencian por presencia de heartbeat
reciente. Si el lease vence → `backlog`.

## Catálogo de endpoints (a crear)

- `POST /backlog/claim` — el worker pide la próxima card disponible.
  Body: `{agentID, capabilities?}`. Response: `{cardID, slug, body}` o `204`.
  Marca la card como `claimed` con lease de N minutos.
- `POST /backlog/<id>/heartbeat` — el worker renueva el lease.
- `POST /backlog/<id>/update` — el worker reporta progreso o
  resultado. Body: `{status, diff?, note?, resultURL?}`. Solo
  válido si la card está `claimed` o `running` por ese `agentID`.
- `GET /backlog/queue?status=backlog&priority=P1` — vista opcional
  para debugging, sin claim.

## Cambios mínimos necesarios

1. **Card model** — agregar campos `AssignedTo string`, `ClaimedAt
   time.Time`, `LeaseUntil time.Time`, `LastHeartbeat time.Time` en
   `internal/backlog/application/card.go`. Migrar el FS store
   (`internal/backlog/infrastructure/fs/store.go`) para leer/escribir
   estos campos en el frontmatter.
2. **Lock por card** — el `DistributedLock` de `internal/scheduler/`
   se puede reusar por `cardID` para evitar que dos workers la
   agarren a la vez (defensa en profundidad, no es lo principal).
3. **Servicio de claim** — nuevo en `internal/backlog/application/`:
   `DispatchService` con `Claim(ctx, agentID)`, `Heartbeat`,
   `Update`. Usa el `Repository` existente.
4. **Handlers HTTP** — en `internal/backlog/http/` (o nuevo paquete
   `internal/backlog/dispatch/`). Registrar con `ioc.Register(...)`.
   Proteger con middleware que valide `kind=agent` en el JWT.
5. **Tests** — unitarios del `DispatchService` con fake repo, e
   integración del flujo claim → heartbeat → update → done.
6. **Card de ejemplo** — crear una card P3 en
   `internal/backlog/board/backlog/` para que un worker de prueba
   la tome y valide el flujo end-to-end.

## MVP ultra-chico para validar la idea

Sin scheduler, sin distributed lock por card, sin interfaz rica.
Solo:

```sh
# worker.sh (en otro proceso o VM)
while true; do
  CARD=$(curl -sX POST http://host:8001/backlog/claim \
    -H "Authorization: Bearer $AGENT_JWT" \
    -d '{"agentID":"worker-1"}')
  if [ -n "$CARD" ]; then
    # ejecutar (otro pi, un script, lo que sea)
    # ...
    curl -sX POST "http://host:8001/backlog/$CARD_ID/update" \
      -H "Authorization: Bearer $AGENT_JWT" \
      -d '{"status":"done","note":"...","diff":"..."}'
  fi
  sleep 30
done
```

Si esto sirve, después se pule:
- Scheduler que mire la queue y despache.
- Lease automático con expiración.
- Métricas / dashboards.
- Asignación por capabilities (no solo FIFO).

## Lo que NO hace este card

- Scheduler automático que mire la queue. **Out of scope MVP.**
- Reasignación dinámica de agents por carga. **Out of scope.**
- UI nueva para "ver workers activos". El board actual sirve.
- Proposer LLM ni detección de intención. Sigue siendo chat explícito.
- Multi-tenancy de workers. Todos los agents comparten el mismo
  pool de cards por ahora.

## Acceptance Criteria

- [ ] `Card` tiene `AssignedTo`, `ClaimedAt`, `LeaseUntil`,
      `LastHeartbeat` y se persisten en el FS store.
- [ ] Estados nuevos `claimed`, `running`, `review`, `blocked`
      registrados en `Status.Valid()`.
- [ ] `DispatchService` con `Claim`, `Heartbeat`, `Update`
      implementado y testeado unitariamente.
- [ ] `POST /backlog/claim` retorna la próxima card P1 backlog y la
      marca como `claimed` con lease.
- [ ] `POST /backlog/<id>/heartbeat` renueva el lease y rechaza
      si la card no está claimed por ese agentID.
- [ ] `POST /backlog/<id>/update` acepta transición válida
      (`claimed→running`, `running→done`, etc.) y rechaza inválidas
      con 409.
- [ ] Middleware de auth permite JWTs con `kind=agent` además de
      los `allowedAppEmails` actuales.
- [ ] Test de integración claim → heartbeat → update → done, con
      un worker fake en proceso.
- [ ] Card de ejemplo en `internal/backlog/board/backlog/` lista
      para ser tomada por un worker de prueba.
- [ ] Documentación mínima en `doc/dispatch.md` con el protocolo
      y un ejemplo de worker.

## Dependencias

- `internal/backlog/` (existente, extender).
- `internal/scheduler/DistributedLock` (reusable para lease por card).
- `internal/auth/` (extender middleware para `kind=agent`).
- NO depende de `internal/supervisor/` (que no existe y no se va a
  construir).

## Links

- Reemplaza: `supervisor-reactivo-opt-in-para-sesiones-del-agente.md`
  (deprecada).
- Inspirado en: el flujo *"Mover al hablar"* de
  `doc/backlog-workflow.md` (regla 5).
- Patrón de apagado limpio replicable: `AGENT_ENABLED`.

## Examples

### Flujo esperado

```
[otro proceso pi / VM]
worker-1 → POST /backlog/claim
server   → card #43 "refactor parser fechas" [P1]
worker-1 → POST /backlog/43/heartbeat (cada 5min)
worker-1 → ejecuta pi en su sandbox
worker-1 → POST /backlog/43/update {status: "done",
                                     diff: "...",
                                     note: "tests OK"}
```

### Card de ejemplo sembrada

```markdown
---
priority: P3
status: backlog
tags: [dispatch-test, smoke]
title: 'Dispatch smoke test'
type: backlog/card
---

# Dispatch smoke test

Card tonta para validar que un worker puede tomarla, hacer heartbeat
y reportar `done`. No requiere implementación real.
```
