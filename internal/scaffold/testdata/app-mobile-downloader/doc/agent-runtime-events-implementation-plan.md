# Plan implementable: runtime events persistentes en PostgreSQL

## Objetivo

Eliminar la inconsistencia entre:

- el stream SSE que ve el browser,
- la materialización de `/history`,
- y la rehidratación tras refresh o reconexión.

La estrategia es mover el source of truth del runtime del chat a PostgreSQL,
manteniendo SSE como transporte.

---

## Decisiones cerradas

### 1. Payload inicial

En la primera iteración **no normalizar** los eventos.

Persistir el envelope actual tal como ya existe en el sistema:

- `kind = "pi"`
- `payload = agentapp.Event` completo en `jsonb`

Esto evita introducir una segunda semántica de eventos mientras todavía se
está cerrando la invariancia básica del sistema.

### 2. Contrato cliente

En la primera fase **no crear** todavía un endpoint nuevo tipo:

- `/runtime?offset=N`

Se mantiene el contrato actual del browser:

- `GET /agent/sessions/{id}/events`
- `GET /agent/sessions/{id}/history`

### 3. Estrategia de migración

No migrar lectura y escritura al mismo tiempo.

Primero:

- escribir en Postgres en paralelo,
- mantener lectura actual.

Luego:

- mover `/history` a Postgres,
- después mover replay/reconexión,
- y al final retirar file-based persistence.

### 4. Source of truth por fase

- **Fase 1** → write dual (archivo + Postgres), read legacy
- **Fase 2** → write dual, read `/history` desde Postgres
- **Fase 3** → replay SSE desde Postgres
- **Fase 4** → Postgres como única verdad

---

## Problema que resuelve

Síntomas actuales observados:

- el chat se corta,
- refresh pierde parte de la conversación,
- scroll puede desalinear lo visible,
- `lastEventId` del browser puede quedar por delante de `history.lastSeq`,
- la UI puede mostrar texto que después no logra reaparecer.

Ejemplo real observado:

- `lastEventId = 422`
- `history.lastSeq = 419`

Esto indica que el browser recibió más eventos de los que `/history`
representó finalmente.

---

## Principio de diseño

La conversación no debe depender del stream ni del DOM del browser.

La nueva regla del sistema es:

> Todo evento relevante del runtime debe persistirse primero o de forma
> equivalente en una verdad durable; luego puede transmitirse por SSE,
> rehidratarse como history o recuperarse tras reconexión.

En resumen:

- **SSE = transporte**
- **PostgreSQL = verdad**
- **history = proyección**

---

## Modelo de datos

## Tabla `agent_runtime_events`

Schema mínimo inicial:

```sql
create table agent_runtime_events (
  id bigserial primary key,
  session_id text not null,
  offset bigint not null,
  kind text not null default 'pi',
  payload jsonb not null,
  created_at timestamptz not null default now(),
  unique (session_id, offset)
);

create index agent_runtime_events_session_created_at_idx
  on agent_runtime_events(session_id, created_at);

create index agent_runtime_events_session_offset_idx
  on agent_runtime_events(session_id, offset);
```

### Notas

- `offset` es monotónico por `session_id`.
- `payload` guarda el envelope actual completo del evento.
- No se introduce aún una tabla de proyección separada.

---

## Ubicación sugerida en el repo

### Migración SQL

- `db/migrations/..._create_agent_runtime_events.sql`

### Infraestructura

- `pkg/agent/infrastructure/postgresql/runtime_events.go`

### Application

- `pkg/agent/application/history.go`
  - refactor parcial, o
- `pkg/agent/application/runtime_history.go`
  - si conviene separar la proyección desde eventos

### HTTP

- `pkg/agent/http/events.go`
- `pkg/agent/http/sessions.go`

---

## Interfaz mínima del repositorio

```go
type RuntimeEventsRepository interface {
    Append(ctx context.Context, sessionID string, kind string, payload any) (offset uint64, err error)
    ListAfter(ctx context.Context, sessionID string, after uint64, limit int) ([]RuntimeEventRow, error)
    ListSession(ctx context.Context, sessionID string, before uint64, limit int) ([]RuntimeEventRow, error)
}
```

`RuntimeEventRow` mínimo:

```go
type RuntimeEventRow struct {
    Offset    uint64
    SessionID string
    Kind      string
    Payload   json.RawMessage
    CreatedAt time.Time
}
```

---

## Fase 1 — Persistencia paralela en Postgres

## Objetivo

Agregar durabilidad real sin romper el contrato actual.

## Cambios

1. Crear la migración SQL de `agent_runtime_events`.
2. Implementar repositorio PostgreSQL.
3. Modificar `pkg/agent/http/events.go` para que cada evento SSE:
   - siga la ruta actual,
   - y además se appendée a Postgres.
4. Mantener temporalmente:
   - `tmp/agent-events`
   - `tmp/agent-transcripts`

## Criterio de aceptación

- cada evento emitido al browser queda también en Postgres;
- el flujo actual del chat no se rompe;
- `go build ./...` sigue pasando.

## Resultado esperado

Ya se puede comparar:

- lo que vio el browser,
- lo que `/history` devuelve,
- y lo que Postgres registró.

Esto cierra el diagnóstico con evidencia real.

---

## Fase 2 — `/history` desde Postgres

## Objetivo

Que el refresh dependa de la nueva verdad durable.

## Cambios

1. Implementar reconstrucción de `ConversationHistory` desde `agent_runtime_events`.
2. Ajustar `pkg/agent/http/sessions.go` para que `/history` lea desde Postgres.
3. Mantener transcript file-based solo como fallback temporal si hiciera falta.

## Criterio de aceptación

- refresh rehidrata desde Postgres;
- desaparece el gap principal entre browser e history en casos normales;
- `history.lastSeq` converge con el último evento relevante mostrado.

---

## Fase 3 — Replay SSE desde Postgres

## Objetivo

Que reconexión y continuidad dependan de Postgres, no de jsonl.

## Cambios

1. En `pkg/agent/http/events.go`, resolver replay inicial desde Postgres.
2. Mantener el endpoint actual `/events`.
3. Reusar `Last-Event-ID` / `resume` sobre offsets persistidos en DB.

## Criterio de aceptación

- una reconexión SSE no pierde eventos;
- el browser puede retomar desde su último offset conocido;
- `lastEventId` vuelve a tener correlación fuerte con la verdad persistida.

---

## Fase 4 — Retiro de file-based persistence

## Objetivo

Eliminar duplicidad y complejidad accidental.

## Cambios

1. Quitar `tmp/agent-events` como source of truth.
2. Quitar `tmp/agent-transcripts` como source of truth.
3. Simplificar paths legacy de materialización file-based.

## Criterio de aceptación

- existe una sola verdad: Postgres;
- refresh, replay y history salen del mismo sistema;
- se reducen los paths donde puede aparecer inconsistencia.

---

## Orden recomendado de PRs

### PR 1

**Persistencia paralela**

- migración SQL
- repo Postgres
- append desde `pkg/agent/http/events.go`

### PR 2

**`/history` desde Postgres**

- proyección/reconstrucción
- sessions/history leyendo DB

### PR 3

**Replay SSE desde Postgres**

- `Last-Event-ID`
- replay desde offsets persistidos

### PR 4

**Retiro del legacy file-based**

- limpieza de `tmp/agent-events`
- limpieza de `tmp/agent-transcripts`
- simplificación de código

---

## Qué no hacer por ahora

### No tocar `sync-ai-gateway` todavía

La evidencia actual apunta a inconsistencia local en `scaffoldxd1`, no a una causa
raíz primaria en el gateway.

### No meter ElectricSQL todavía

ElectricSQL puede ser útil más adelante, pero hoy meterlo antes de cerrar la
invariancia básica añadiría complejidad sin resolver primero el bug real.

### No cambiar el cliente al principio

No agregar `/runtime?offset=` en la primera fase.

Primero estabilizar persistencia bajo el contrato actual.

---

## Riesgos aceptados

### Riesgo 1 — dual write temporal

Durante un tiempo habrá escritura en dos lados:

- file-based legacy
- Postgres nuevo

Se acepta porque permite una migración segura por etapas.

### Riesgo 2 — más writes a Postgres

Cada evento append-only aumenta el tráfico de DB.

Se acepta en la primera fase para priorizar consistencia y debugging.
Optimizaciones de compactación vienen después.

### Riesgo 3 — proyección on-the-fly inicial

La reconstrucción de history desde eventos puede ser más costosa al inicio.
Se acepta en Fase 2 y se optimiza solo si duele de verdad.

---

## Política de retención futura

No resolverla en la primera iteración.

Opciones futuras:

1. TTL por días,
2. compactación por turno,
3. snapshots de assistant final + prune de deltas viejos.

No es requisito previo para Fase 1.

---

## Criterios de aceptación globales

El plan se considera exitoso si:

1. el texto visible del agente no desaparece tras refresh;
2. `/history` refleja consistentemente lo que el browser alcanzó a ver;
3. reconexión SSE ya no depende de archivos locales frágiles;
4. existe observabilidad clara sobre qué eventos llegaron, se persistieron y se proyectaron;
5. deja de ser necesario usar el DOM o el estado JS como única fuente de diagnóstico.

---

## Primer corte recomendado

El primer corte útil y barato es:

1. migración SQL `agent_runtime_events`
2. repositorio PostgreSQL
3. append desde `pkg/agent/http/events.go`

Eso ya entrega valor sin reescribir aún `/history` ni el cliente.

---

## Siguiente paso sugerido

Antes de implementar, validar solo estas dos decisiones operativas:

1. ¿`offset` se calcula como `max(offset)+1` por `session_id` o se delega a SQL con una estrategia explícita por sesión?
2. ¿Fase 2 reconstruye `history` on-the-fly desde eventos o introduce ya una proyección materializada?

Con esas dos decisiones cerradas, el plan queda listo para bajar a código.
