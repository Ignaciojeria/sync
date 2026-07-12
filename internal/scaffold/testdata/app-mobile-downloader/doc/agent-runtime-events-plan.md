# Plan: runtime events persistentes en PostgreSQL

## Objetivo

Resolver la inconsistencia entre:

- lo que el browser ve por SSE,
- lo que `/history` logra materializar,
- y lo que reaparece al refresh o reconexión.

La mejora propuesta es cambiar el source of truth del chat del agente:

- **antes**: SSE + jsonl/transcript local como mecanismo principal
- **después**: **PostgreSQL como source of truth append-only**
- **SSE queda solo como transporte**

---

## Problema actual

Hoy el proyecto presenta síntomas como:

- el stream se corta,
- el refresh pierde parte de la conversación,
- el scroll puede desalinear lo visible,
- `lastEventId` del browser puede quedar por delante de `history.lastSeq`,
- la UI puede mostrar texto que luego no se puede rehidratar correctamente.

Ejemplo observado:

- `lastEventId = 422`
- `history.lastSeq = 419`

Esto indica que el browser recibió más eventos de los que el transcript/history
persistido terminó representando.

---

## Principio de diseño

La conversación no debe depender del stream en vivo ni del DOM del browser.

La regla deseada es:

> Todo evento relevante del runtime debe persistirse primero como evento
> append-only; luego puede transmitirse por SSE, reconstruirse como history,
> o rehidratarse tras refresh.

En esta arquitectura:

- **SSE = transporte**
- **PostgreSQL = verdad**
- **history = proyección**

---

## Modelo conceptual

En vez de pensar solo en “chat”, pensar en:

## Session Runtime

Una sesión contiene un log append-only de eventos del runtime, por ejemplo:

- prompt del usuario,
- deltas del asistente,
- tool calls,
- tool results,
- stderr,
- errores del runtime,
- cierre de turno,
- cierre del agente.

No todos esos eventos se renderizan igual al usuario, pero todos forman parte del
runtime real de la sesión.

---

## Tabla propuesta

## `agent_runtime_events`

Campos mínimos sugeridos:

- `id` — `bigserial primary key`
- `session_id` — `text not null`
- `offset` — `bigint not null`
- `kind` — `text not null`
- `payload` — `jsonb not null`
- `created_at` — `timestamptz not null default now()`

### Restricciones

- unique `(session_id, offset)`

### Índices

- index `(session_id, created_at)`
- index `(session_id, offset)`

---

## Tipos de evento iniciales

No hace falta sobre-modelar desde el día 1. Se puede empezar con:

- `user_prompt`
- `assistant_delta`
- `tool_start`
- `tool_result`
- `stderr`
- `runtime_error`
- `turn_end`
- `agent_end`
- `runtime_exit`

Opcionalmente, si se quiere conservar el envelope actual de `pi`, se puede usar:

- `kind = pi_event`
- `payload = Event` completo

Y derivar los tipos semánticos en la capa application.

---

## Estrategia recomendada

No migrar todo de una vez.

Hacerlo por fases.

---

## Fase 1 — Persistencia paralela

## Objetivo

Mantener el flujo actual, pero agregar persistencia confiable en PostgreSQL.

## Cambios

1. Crear la tabla `agent_runtime_events`.
2. Crear un repositorio PostgreSQL para runtime events.
3. En `pkg/agent/http/events.go`, cada evento emitido por SSE debe:
   - persistirse en Postgres,
   - obtener un `offset`,
   - seguir enviándose al browser.
4. Mantener temporalmente:
   - `tmp/agent-events`
   - `tmp/agent-transcripts`

## Beneficio

- no rompe el flujo actual,
- permite comparar lo que ve el browser vs lo que queda persistido,
- habilita observabilidad real.

---

## Fase 2 — History desde Postgres

## Objetivo

Hacer que `/history` ya no dependa de jsonl/transcript file-based.

## Cambios

1. Implementar reconstrucción de `ConversationHistory` desde `agent_runtime_events`.
2. Hacer que `GET /agent/sessions/{id}/history` lea desde Postgres.
3. Mantener transcript file-based solo como fallback temporal, si hiciera falta.

## Beneficio

- refresh depende de la misma verdad que usa el runtime,
- se elimina parte del desfase entre stream y rehidratación.

---

## Fase 3 — Replay por offset

## Objetivo

Que la reconexión no dependa de memoria local o jsonl.

## Cambios

1. Agregar endpoint:
   - `GET /agent/sessions/{id}/runtime?offset=N`
2. O reutilizar SSE con replay inicial leyendo desde Postgres.
3. El browser mantiene `lastOffset` en vez de confiar solo en el DOM.
4. Al reconectar:
   - se piden los eventos `offset > lastOffset`

## Beneficio

- reconexión robusta,
- menos dependencia del timing del stream,
- mejor base para debugging y auditoría.

---

## Fase 4 — Retiro de file-based persistence

## Objetivo

Eliminar la fuente de duplicidad y complejidad accidental.

## Cambios

1. Retirar `tmp/agent-events` como source of truth.
2. Retirar `tmp/agent-transcripts` como source of truth.
3. Dejar archivos locales solo si siguen sirviendo como cache o debug local.

## Beneficio

- una sola verdad,
- menos paths de fallo,
- menos inconsistencia entre stream/history/UI.

---

## Ubicación sugerida en el repo

### Infraestructura

Crear:

- `pkg/agent/infrastructure/postgresql/runtime_events.go`

Responsabilidades:

- `AppendEvent(...)`
- `ListEventsAfter(...)`
- `ListSessionEvents(...)`

### Application

Agregar lógica en:

- `pkg/agent/application/`

Responsabilidades:

- reconstruir history desde eventos,
- proyectar assistant/tool/error a `ConversationHistory`.

### HTTP

Ajustar:

- `pkg/agent/http/events.go`
- `pkg/agent/http/sessions.go`

Responsabilidades:

- append por evento,
- replay por offset,
- history desde Postgres.

---

## Contrato HTTP futuro sugerido

### Transporte / runtime

- `GET /agent/sessions/{id}/runtime?offset=N`

Devuelve eventos append-only desde un offset.

### Vista de conversación

- `GET /agent/sessions/{id}/history`

Devuelve proyección amigable para UI.

### SSE

Puede mantenerse como:

- `GET /agent/sessions/{id}/events`

pero leyendo/reproduciendo desde Postgres, no desde jsonl local.

---

## Qué NO hacer por ahora

### No empezar por `sync-ai-gateway`

El gateway puede gatillar cortes, pero no es la causa raíz principal del bug
actual.

La evidencia observada apunta más a:

- persistencia local inconsistente,
- materialización parcial,
- rehidratación defectuosa en `testboi1`.

### No meter ElectricSQL todavía

ElectricSQL puede servir en el futuro para sincronización más sofisticada,
pero hoy añadiría complejidad antes de cerrar la invariancia básica:

> lo que el usuario vio debe poder reaparecer tras refresh.

---

## Riesgos / tradeoffs

### Ventajas

- refresh confiable,
- reconexión confiable,
- source of truth claro,
- debugging mucho más simple,
- preparación para futuros durable streams.

### Costos

- más writes a Postgres,
- nueva tabla de eventos con crecimiento continuo,
- necesidad futura de retención o compactación,
- una capa extra de proyección para construir history.

---

## Política de retención sugerida

No resolverla de inmediato, pero dejarla prevista.

Opciones futuras:

1. retener todos los eventos por ventana corta (ej. 7–30 días),
2. compactar eventos viejos en transcript materializado,
3. borrar deltas intermedios dejando snapshots por turno.

No hace falta resolver esto antes de la Fase 1.

---

## Criterios de aceptación

Se considera exitoso si:

1. el texto visible del agente no desaparece tras refresh;
2. `history.lastSeq` converge al último evento relevante mostrado al browser;
3. el navegador puede reconectar usando `offset` sin perder eventos;
4. `/history` y el stream dejan de divergir en casos normales;
5. deja de ser necesario depender del estado efímero del DOM para diagnosticar sesiones.

---

## Primer corte recomendado

El primer corte útil y barato es:

1. migración SQL `agent_runtime_events`
2. repositorio PostgreSQL en `pkg/agent/infrastructure/postgresql/`
3. append desde `pkg/agent/http/events.go`

Ese corte ya entrega valor sin reescribir todo el chat.

---

## Siguiente paso sugerido

Antes de implementar, responder estas 3 decisiones:

1. ¿`offset` será global por sesión o `bigserial` por fila + proyección por `session_id`?
2. ¿persistimos el envelope `Event` actual completo o lo normalizamos a tipos más semánticos desde el inicio?
3. ¿`/history` se reconstruirá on-the-fly desde eventos o se mantendrá una proyección materializada?

Con esas tres decisiones cerradas, el plan queda listo para bajar a código.
