# Plan de mejora UX del chat del agente

## Objetivo

Mejorar la experiencia conversacional del chat embebido del agente para que el
usuario siempre entienda qué está pasando durante un turno, especialmente en
casos de:

- respuesta larga,
- uso de herramientas,
- reconexión SSE,
- interrupción del stream,
- errores tardíos o ambiguos,
- respuestas truncadas visualmente.

La meta no es cambiar la arquitectura base del proyecto, sino endurecer la
**máquina de estados UX** del chat para que transmita confianza y evite falsos
positivos o silencios.

---

## Estado del plan (2026-07-08)

- **Fase 1** — `[x]` Implementada en `pkg/agent/ui/page.templ` (errores diferidos,
  thinking hasta cierre real, placeholder al fondo, sellado desde `/history`).
- **Fase 2** — `[x]` Implementada (banner de estado, watchdog 10 s, botones
  `Continuar`/`Reintentar`, auto-reconexión con backoff).
- **Fase 3** — `[x]` Implementada implícitamente: 5-6 variables explícitas
  (`turnIsOpen`, `currentThinkingBubble`, `currentAssistantBubble`,
  `turnHasVisibleAssistantText`, `pendingTerminalError`, `lastTurnActivityAt`)
  + `setConnectionState` cumplen el rol del state machine. No se introduce
  un `phase` formal — es redundante.
- **Validación pendiente** — ejecutar los 6 escenarios de
  [Siguiente paso sugerido](#siguiente-paso-sugerido) en VM. Si ninguno
  falla, el plan queda cerrado.

> **Nota:** durante la implementación aparecieron decisiones que el plan
> original no cubría. Están documentadas en
> [Implementaciones вне del plan original](#implementaciones-fuera-del-plan-original).

---

## Problemas observados

### 1. [x] Errores efímeros / falsos positivos

A veces la UI muestra `Ocurrió un error.` aunque el agente termina respondiendo
bien. Esto ocurre cuando se renderiza un evento ambiguo o transitorio como si
fuera un error terminal.

### 2. [x] Burbuja de thinking inconsistente

La burbuja de thinking a veces desaparece antes de que termine realmente la
respuesta o queda desalineada respecto del último bloque visible del asistente.

### 3. [x] Respuestas truncadas visualmente

El stream puede terminar correctamente (`turn_end` / `agent_end`), pero la UI
puede quedarse con una versión parcial del texto si faltó algún `text_delta` o
si hubo un repintado intermedio.

### 4. [x] Recuperación deficiente ante cortes

Cuando la sesión se interrumpe, el usuario a veces no recibe feedback claro y
termina probando manualmente con mensajes como `continua` o `ping`.

### 5. [x] Estado del turno poco visible

La UI no siempre comunica de forma explícita si el turno está:

- pensando,
- ejecutando herramientas,
- redactando,
- reconectando,
- interrumpido,
- finalizado.

---

## Principios UX a aplicar

1. **Una respuesta visible gana sobre un error tardío.**
2. **El usuario nunca debe quedarse sin feedback.**
3. **El turno debe tener un fin visual inequívoco.**
4. **La respuesta en progreso debe ocupar siempre el último lugar visible del feed.**
5. **La UI debe recuperarse sola antes de pedir intervención manual.**
6. **Los errores solo deben mostrarse cuando el turno realmente terminó mal.**

---

## Alcance del plan

Este plan se enfoca en la capa:

- `pkg/agent/ui/page.templ`
- contrato visual de eventos SSE consumidos por la UI

No requiere cambios en:

- `sync-ai-gateway`
- contrato OpenAI-compatible del upstream
- topología de 3 procesos

Podría requerir ajustes menores en worker sólo si se detecta ambigüedad real en
la semántica de eventos internos, pero la primera fase es 100% UX/UI local.

---

## Estado objetivo del chat

Cada turno del chat debe recorrer estados UX claros:

```text
idle
  → thinking
  → tooling (opcional)
  → answering
  → sealed
```

Y en casos anómalos:

```text
thinking/tooling/answering
  → reconnecting
  → interrupted
  → retrying / resumed
```

Y en fallo real:

```text
thinking/tooling/answering
  → failed
```

---

## Plan por fases

# Fase 1 — Estabilización visual del turno `[x]`

## Objetivo

Eliminar los síntomas más visibles e inconsistentes:

- errores falsos positivos,
- thinking que desaparece antes de tiempo,
- respuesta truncada,
- tools desordenando el final visible.

## Cambios

### 1. [x] Error terminal diferido

No mostrar errores intermedios al instante.

Regla:

- `message_start.errorMessage`, `stderr`, `runtime_error` → guardar como
  `pendingTerminalError`
- solo mostrar error en `turn_end` / `agent_end` si **no hubo texto visible**
- si hubo `text_delta`, descartar el error pendiente

### 2. [x] Thinking hasta cierre real del turno

No ocultar thinking con `message_end`.

Regla:

- ocultar thinking solo con:
  - `turn_end`
  - `agent_end`
  - `runtime_exit` real

### 3. [x] Placeholder del asistente al final

Mantener una única respuesta activa del asistente al final del feed.

Regla:

- tools se insertan arriba de la respuesta activa
- la respuesta en progreso siempre queda abajo
- el último lugar visible del feed es siempre el bloque activo del asistente

### 4. [x] Sellado final desde history

En `turn_end` / `agent_end`, reconciliar el mensaje final con `/history`.

Objetivo:

- evitar que quede una versión visual truncada
- usar el transcript persistido como fuente final de verdad

## Criterio de aceptación

- nunca aparece `Ocurrió un error` si la respuesta terminó bien
- la burbuja de thinking no desaparece antes del fin del turno
- la respuesta final visible coincide con la persistida en history
- tools no desplazan la respuesta activa fuera del fondo del feed

---

# Fase 2 — Recuperación explícita ante interrupciones `[x]`

## Objetivo

Evitar que el usuario tenga que adivinar si el chat sigue vivo.

## Cambios

### 1. [x] Banner de estado del turno

Agregar un banner visible con estados como:

- `Reconectando la respuesta…`
- `La respuesta se interrumpió.`
- `Intentando recuperar la respuesta…`
- `Reintentando…`

### 2. [x] Watchdog de actividad

Si un turno sigue abierto y no hay actividad por ~10s:

- marcar estado `interrupted`
- mostrar banner con CTA

### 3. [x] Botón `Continuar`

Acción esperada:

- reintentar conexión SSE
- reconciliar el transcript reciente
- mantener la sesión sin obligar al usuario a escribir manualmente

### 4. [x] Botón `Reintentar`

Acción esperada:

- reenviar el último prompt conocido
- mostrar feedback inmediato

### 5. [x] Nunca silencio

Si el usuario intenta seguir conversando y no hay stream activo:

- la UI debe mostrar feedback de recuperación, nunca quedarse muda

## Criterio de aceptación

- si el turno se corta, el usuario ve feedback en menos de 10s
- hay una acción visible para recuperar o reintentar
- no hace falta usar `continua` o `ping` manualmente para diagnosticar el estado

---

# Fase 3 — Máquina de estados UX formalizada `[x] (implícita)`

## Objetivo

Dejar el chat con un modelo explícito y mantenible, en vez de condicionales
dispersos por evento.

## Decisión de implementación

En lugar de introducir un `phase` formal redundante, se consolidó el estado
en 5-6 variables explícitas en `page.templ` que cubren los mismos casos:

| Plan original | Variable real |
|---|---|
| `phase: "idle" \| "thinking" \| ...` | `currentThinkingBubble !== null` + `currentAssistantBubble` |
| `isOpen: boolean` | `turnIsOpen` |
| `hasVisibleAssistantText: boolean` | `turnHasVisibleAssistantText` |
| `pendingTerminalError: string` | `pendingTerminalError` |
| `lastActivityAt: number` | `lastTurnActivityAt` |
| `lastUserPrompt: string` | `lastUserPrompt` |
| `sealedFromHistory: boolean` | Implícito: `sealAssistantResponseFromHistory` se ejecuta en `turn_end`/`agent_end` |

## Beneficio

- cero carreras entre SSE e history (las decisiones de sellado y error
  usan el mismo flag `turnHasVisibleAssistantText`),
- decisiones UX previsibles (un solo path por tipo de evento terminal),
- `setConnectionState` cubre `connected`/`connecting`/`reconnecting`/`disconnected`
  vía `data-connection-state` en el root.

## Criterio de aceptación

- [x] cada turno tiene un estado único y trazable
- [x] se puede instrumentar con logs sin ambigüedad (`console.log` por
  transición en `handleStreamPayload`)
- [x] la UI deja de "adivinar" el estado desde múltiples señales sueltas

---

## Recomendaciones de implementación

## Prioridad recomendada

1. Fase 1 completa
2. Fase 2 completa
3. Fase 3 si sigue habiendo complejidad accidental

## Estrategia de rollout

### Paso 1

Aplicar quick wins en `pkg/agent/ui/page.templ`.

### Paso 2

Validar en VM con sesiones reales:

- prompt corto,
- prompt largo,
- prompt con muchas tools,
- reconexión manual,
- caída simulada del worker,
- refresh del navegador a mitad de turno.

### Paso 3

Agregar logs temporales de diagnóstico si queda alguna carrera:

- evento SSE recibido,
- fase actual del turno,
- transición de estado,
- decisión de sellado / error / reconexión.

---

## Métricas cualitativas para evaluar la mejora

Se considera exitoso si el usuario percibe que:

- el chat “siempre sabe” si está trabajando o interrumpido,
- nunca aparecen errores fantasma cuando la respuesta fue exitosa,
- no necesita probar manualmente con `ping` o `continua`,
- el final de la respuesta queda estable y legible,
- el orden visual de tools + thinking + respuesta se siente natural.

---

## Implementaciones fuera del plan original

Durante la implementación aparecieron cuatro decisiones que el plan original
no cubría y que son las que terminaron eliminando los casos de `ping`/`continua`
manuales:

### 1. Auto-reconexión con backoff exponencial + jitter

`keepStreamAlive` envuelve `connectStream` en un loop infinito con backoff
1s → 2s → 4s → … → 30s + jitter ±20%. Sin esto, una caída de 30 s del worker
producía 30 requests a `/agent/auth` y `/events`. Con backoff: ~5 requests.

Justificación: el boilerplate corre 3 procesos (BFF, web-server, agent-worker);
reiniciar el worker no debe tumbar la UX. La reconexión silenciosa + banner
"Reconectando…" es la única forma de cumplir "nunca silencio" sin spamear
al IdP.

### 2. Reanudación de stream con `Last-Event-ID` + `?resume=N`

Si la conexión SSE se cae (network blip, tab suspendida), el cliente guarda
el último `id` recibido en `lastEventIds[sessionId]` y al reabrir lo manda
como header `Last-Event-ID` Y como query `?resume=N` (fallback para proxies
que filtran el header, ej. algunos BFFs). El worker reenvía los eventos
perdidos y la UI los procesa en orden.

Caso especial: si todavía no hay `lastId`, se conecta con `?resume=live`
para NO replayear backlog. El historial visible lo carga `/history` aparte.

### 3. Saneado de errores genéricos persistidos

`isGenericHistoryError` + `dropRecentGenericErrorBubbles` filtran dos casos:

- un error `runtime_error` persistido en `/history` ANTES de que llegara la
  corrección del error diferido,
- errores genéricos tipo `"Ocurrió un error."` o `"500 \"internal_error\""`.

Si hay un assistant con texto no vacío después de un error genérico, el
error se borra del render. Esto evita que history viejo contamine sesiones
nuevas.

### 4. Retry 401 con refresh forzado de JWT

Tanto `connectStream` como `postPrompt` reintentan una vez con
`getAuthToken(true)` si el primer intento devuelve 401. Esto cubre el caso
típico: el JWT embebido en `data-user-jwt` venció mientras el tab estaba
abierto, y el usuario envía un prompt sin haber recargado la página.

Sin esto, el primer prompt post-expiración mostraba "401" en banner y el
usuario tenía que refrescar manualmente.

---

## Opinión general sobre el proyecto

> _Sección removida: relleno descriptivo sin acción concreta. La justificación
> técnica del stack vive en `AGENTS.md` y en `doc/agent-runtime.md`._

---

## Siguiente paso sugerido

Ejecutar y validar manualmente los 6 escenarios en VM con el worker real.
**Si todos pasan, el plan se da por cerrado.** Anotar acá el resultado.

| # | Escenario | Resultado |
|---|---|---|
| 1 | respuesta corta sin tools | _pendiente_ |
| 2 | respuesta larga con varias tools | _pendiente_ |
| 3 | refresh en medio del turno | _pendiente_ |
| 4 | worker detenido y reconectado | _pendiente_ |
| 5 | error terminal real sin texto visible | _pendiente_ |
| 6 | respuesta exitosa con evento ambiguo tardío | _pendiente_ |
