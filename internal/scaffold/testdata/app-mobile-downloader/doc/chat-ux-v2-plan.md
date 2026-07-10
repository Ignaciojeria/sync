# Plan v2 de mejora UX del chat del agente

## Objetivo

Mejorar la **UX cognitiva del turno** del chat del agente.

La base técnica del stream ya está bastante resuelta en `pkg/agent/ui/page.templ`:

- error terminal diferido,
- thinking hasta cierre real,
- respuesta activa al fondo,
- sellado final desde `/history`,
- reconexión con watchdog,
- botones `Continuar` / `Reintentar`.

El problema actual ya no es principalmente de SSE o transporte, sino de
**comportamiento del agente durante el turno**:

- responde demasiado largo por defecto,
- empieza a responder y sigue investigando,
- entrega más de un “final” por turno,
- no siempre comunica con claridad la fase real (`analizando`, `tooling`, `redactando`).

---

## Diagnóstico

La sesión revisada mostró un patrón claro:

1. el agente investigó mucho más de lo necesario para la intención del usuario;
2. empezó a responder antes de cerrar la exploración;
3. volvió a usar tools después de haber comenzado la respuesta visible;
4. emitió varios bloques que se sintieron como respuestas “finales” parciales;
5. al cortarse el stream, la percepción fue de inestabilidad aunque la base SSE ya tenga mecanismos de recuperación.

Resumen:

> La UX técnica del chat mejoró, pero la **UX cognitiva del agente** todavía no.

---

## Alcance

Este plan se enfoca en tres capas concretas:

1. **Policy / prompt del agente**
2. **Disciplina del turno en worker / contrato de eventos**
3. **Estados UX visibles en `pkg/agent/ui/page.templ`**

No busca reescribir:

- `sync-ai-gateway`
- la topología de 3 procesos
- el sistema SSE base ya implementado

---

## Estado objetivo

Cada turno debería sentirse así:

```text
usuario pregunta
  → agente analiza
  → agente usa herramientas (si hace falta)
  → agente redacta una sola respuesta final
  → turno sellado
```

Y no así:

```text
analiza
  → responde un poco
  → vuelve a investigar
  → responde otra vez
  → se corta
  → usuario prueba con “continua” o “ping”
```

---

## Principios UX

1. **Una sola respuesta final visible por turno.**
2. **No investigar después de empezar la respuesta final.**
3. **Breve por defecto; profundo solo si el usuario lo pide.**
4. **La UI debe decir qué está pasando, no solo que “piensa”.**
5. **Si el stream se corta, se recupera el turno; no se delega al usuario.**

---

## Fase A — Disciplina de respuesta del agente

## Objetivo

Reducir verborragia, tool sprawl y múltiples respuestas parciales.

## Cambios

### 1. Respuesta breve por defecto

Agregar una regla explícita en la policy/prompt del agente:

- si el usuario pide opinión general, resumen o contexto, responder corto;
- solo hacer auditoría profunda si el usuario la pide explícitamente;
- si el análisis va a ser largo, responder primero con un resumen y ofrecer ampliar.

### 2. No tool-use después del primer `text_delta`

Definir una regla de turno:

- una vez que el agente empezó a emitir respuesta visible,
- no debería volver a entrar en `tool_execution_start` para ese mismo turno,
- salvo que exista una excepción explícita y visible para el usuario.

### 3. Una sola respuesta final por turno

Aunque el provider emita múltiples submensajes, la UX debe consolidarlos como:

- una única respuesta activa del asistente,
- un único cierre visual,
- una sola percepción de “final”.

### 4. Presupuesto de exploración

Definir una heurística simple:

- máximo N tools / lecturas antes de responder,
- si necesita más exploración, el agente debe pedir permiso o explicitar que hará una auditoría más profunda.

## Alineación con el código actual

### Ya alineado

- `pkg/agent/ui/page.templ` ya soporta una respuesta activa al fondo;
- tools arriba de la respuesta activa;
- sellado final desde history;
- recuperación de stream.

### Gap real

Hoy no hay una regla explícita que imponga:

- “breve por defecto”,
- “no investigar después de empezar a responder”,
- “una sola respuesta final percibida”.

Esto no se resuelve solo en frontend.

---

## Fase B — Estados UX más semánticos

## Objetivo

Hacer visible la fase real del turno.

## Cambios

### 1. Reemplazar thinking genérico por fases visibles

La UI debería distinguir al menos:

- `Analizando…`
- `Leyendo archivos…`
- `Ejecutando herramientas…`
- `Redactando respuesta…`
- `Recuperando respuesta…`

### 2. Mantener el estado visible mientras dure la fase

No esconder demasiado pronto el estado del turno.

La transición deseada es:

```text
thinking/tooling
  → answering
  → sealed
```

### 3. Cierre visual inequívoco

Al cerrar el turno:

- ocultar el estado activo,
- dejar la respuesta sellada,
- evitar la sensación de que “tal vez falta otra parte”.

## Alineación con el código actual

### Ya alineado

La UI ya tiene:

- `statusBanner`
- `showThinking()`
- `hideThinking()`
- `setConnectionState()`
- watchdog de 10 s

### Gap real

Falta enriquecer la semántica visible del estado.

Hoy el sistema sabe bastante bien qué está pasando, pero todavía no siempre lo
expresa con suficiente claridad al usuario.

---

## Fase C — Recuperación perceptible de cortes

## Objetivo

Que un corte no destruya la confianza del usuario en el turno.

## Cambios

### 1. Si hubo texto útil visible, priorizarlo

Si ya hubo respuesta útil:

- no mostrar error genérico como resultado principal,
- mostrar recuperación o interrupción, no fracaso absoluto.

### 2. Sellado desde history + copy de recuperación

Cuando el stream se corta tras haber texto parcial:

- reconciliar desde `/history`,
- mostrar un estado tipo `Respuesta recuperada` o `La respuesta se interrumpió, se recuperó lo generado`.

### 3. Evitar `continua` / `ping`

El usuario no debería tener que diagnosticar manualmente si el chat sigue vivo.

## Alineación con el código actual

### Ya alineado

Esto ya existe en buena parte:

- `pendingTerminalError`
- `turnHasVisibleAssistantText`
- `sealAssistantResponseFromHistory()`
- `connectStream()` con `Last-Event-ID`
- botones `Continuar` / `Reintentar`

### Gap real

El gap no es tanto lógico como de **copy, señalización y cierre visual**.

---

## Tabla: problema → archivo → cambio mínimo → impacto

| Problema | Archivo / capa | Cambio mínimo | Impacto |
|---|---|---|---|
| Respuesta enciclopédica cuando el usuario pidió algo simple | policy / prompt del agente | Regla: breve por defecto; análisis profundo solo si se pide | Alto |
| El agente investiga después de empezar a responder | worker / contrato de eventos | Invalidar o marcar anomalía si hay `tool_execution_start` después del primer `text_delta` | Alto |
| Varias respuestas “finales” en un turno | worker + UI | Consolidar en una sola respuesta activa visible y un solo cierre UX | Alto |
| Thinking demasiado genérico | `pkg/agent/ui/page.templ` | Introducir labels de fase (`Analizando`, `Tooling`, `Redactando`) | Medio/Alto |
| El usuario no sabe si el turno terminó | `pkg/agent/ui/page.templ` | Cierre visual inequívoco del turno | Medio |
| El stream se corta y ensucia una respuesta útil | UI + worker | Priorizar texto visible + copy de recuperación | Medio |
| Demasiadas tools para una pregunta simple | policy / worker | Presupuesto blando de exploración | Alto |
| El usuario termina usando `continua` / `ping` | UI | Mejorar copy + recuperación visible ya existente | Medio |

---

## Prioridad recomendada

### P0

1. **Respuesta breve por defecto**
2. **No tool-use después de empezar a responder**
3. **Estados UX semánticos en `page.templ`**

### P1

4. **Una sola respuesta final por turno**
5. **Presupuesto de exploración**
6. **Mejor copy de recuperación tras socket close**

---

## Orden de implementación sugerido

### Paso 1 — Policy / prompt

Aplicar la regla:

- breve por defecto,
- análisis profundo solo si se pide,
- no seguir investigando después de comenzar la respuesta final.

### Paso 2 — Guard de turno en worker

Agregar la validación:

- si ya hubo `text_delta`, cualquier `tool_execution_start` posterior se considera anomalía o se ignora para UX.

### Paso 3 — Estado visible en UI

En `pkg/agent/ui/page.templ`:

- mostrar labels de fase más explícitos,
- mantenerlos visibles hasta cierre real,
- mejorar copy de recuperación.

---

## Criterios de aceptación

Se considera exitoso si en una sesión real:

1. el agente no entrega una auditoría completa cuando el usuario pidió solo una opinión breve;
2. no aparecen nuevos tools después de que comenzó la respuesta visible;
3. el usuario percibe una sola respuesta final por turno;
4. la UI distingue claramente `analizando`, `ejecutando herramientas` y `redactando`;
5. si hay corte, el usuario ve recuperación clara y no necesita escribir `continua` o `ping`.

---

## Aterrizaje técnico contra el código actual

### 1. Policy / prompt base del agente

**Estado real:** no hay un prompt base explícito en este repo.

### Evidencia

- `pkg/agent/application/manager.go` → `Prompt()` pasa el mensaje del usuario al runtime sin prefijo ni policy local.
- `pkg/agent/infrastructure/pirpc/runner.go` → `runtime.Prompt()` envía `{"type":"prompt","message":...}`.
- `pkg/agent/infrastructure/pirpc/process.go` → el proceso levanta `pi --mode rpc ... -e <extensions>`.
- `.pi/extensions/provider.ts` → registra provider/modelo, no policy conversacional.
- `.pi/extensions/smoke.ts` → smoke test solamente.

### Conclusión

La regla:

- breve por defecto,
- análisis profundo solo si se pide,
- no derivar a auditoría completa sin permiso,

**no tiene hoy un hogar explícito** dentro del proyecto.

### Punto de entrada recomendado

**Archivo candidato:** `pkg/agent/application/manager.go`

### Cambio mínimo recomendado

Implementar un `Steer()` automático una vez por sesión o antes del primer prompt
visible, con una instrucción breve del estilo:

- responder corto por defecto,
- ampliar solo si el usuario lo pide,
- no seguir investigando después de empezar la respuesta final.

### 2. Regla `text_delta` → no más tooling

**Estado real:** hoy no existe esa invariante.

### Evidencia

- `pkg/agent/ui/page.templ` procesa `text_delta` y `tool_execution_start` por separado.
- `pkg/agent/application/history.go` materializa tools y assistant text sin validar el orden del turno.
- `pkg/agent/application/manager.go` hace `broadcast()` raw de eventos, sin normalización.
- `pkg/agent/worker/handlers/events.go` persiste y reenvía el stream tal cual.

### Conclusión

Hoy no hay una capa backend que imponga:

> “si ya comenzó la respuesta visible, no vuelvas a mostrar tooling como parte normal del mismo turno”.

### Punto de entrada más barato

**Archivo candidato:** `pkg/agent/ui/page.templ`

### Cambio mínimo recomendado

Si `turnHasVisibleAssistantText === true` y llega `tool_execution_start`:

- no renderizar la tool como parte normal del turno,
- registrar anomalía en consola,
- mantener la fase en `answering`.

### Punto de entrada más correcto (segunda iteración)

**Archivo candidato:** `pkg/agent/application/manager.go`

Agregar normalización en `broadcast()` con estado por sesión, para detectar y
filtrar `tool_execution_start` después del primer `text_delta`.

### 3. Estados UX semánticos

**Estado real:** la infraestructura ya existe, pero la semántica visible todavía es limitada.

### Evidencia

En `pkg/agent/ui/page.templ` ya existen:

- `statusBanner`
- `statusText`
- `showThinking()` / `hideThinking()`
- `showStatusBanner()`
- `setConnectionState()`
- `handleStreamPayload()`

### Punto de entrada recomendado

**Archivo exacto:** `pkg/agent/ui/page.templ`

### Cambio mínimo recomendado

Agregar una variable JS del tipo:

```js
let turnPhase = "idle";
```

y un helper como:

```js
function setTurnPhase(phase, message) { ... }
```

Mapeo sugerido:

- `message_start` → `Analizando…`
- `tool_execution_start` → `Ejecutando herramientas…`
- primer `text_delta` → `Redactando respuesta…`
- watchdog / reconnect → `Recuperando respuesta…`
- `turn_end` / `agent_end` → limpiar / sellar

---

## Tabla ejecutable: mejora → archivo exacto → cambio mínimo

| Mejora | Archivo exacto | Cambio mínimo |
|---|---|---|
| Respuesta breve por defecto | `pkg/agent/application/manager.go` | `Steer()` automático una vez por sesión o antes del primer prompt |
| No investigar después de empezar a responder (quick win) | `pkg/agent/ui/page.templ` | ignorar/renderizar como anomalía `tool_execution_start` si ya hubo `text_delta` |
| No investigar después de empezar a responder (invariante real) | `pkg/agent/application/manager.go` | normalizar eventos en `broadcast()` o capa intermedia |
| Una sola respuesta final visible | `pkg/agent/ui/page.templ` | consolidar bloques finales como una sola respuesta activa |
| Fases `Analizando / Tooling / Redactando` | `pkg/agent/ui/page.templ` | agregar `turnPhase` + helper de fase |
| Recovery copy más claro | `pkg/agent/ui/page.templ` | ajustar copy de banner cuando hubo texto útil + corte |

---

## Conclusión de implementabilidad

Este plan **sí está listo para implementación**, con una sola salvedad:

- la parte de **policy / prompt** no tiene hoy un hogar explícito en el repo,
  así que hay que decidir si se implementa como `Steer()` automático en
  `manager.go` o como extensión de `pi` en `.pi/extensions/`.

Todo lo demás ya tiene punto de entrada claro y barato.

## Siguiente paso sugerido

Implementar en este orden:

1. `pkg/agent/ui/page.templ` → fases UX semánticas
2. `pkg/agent/ui/page.templ` → guard visual contra tooling después de `text_delta`
3. `pkg/agent/application/manager.go` → policy breve por defecto vía `Steer()`

Con esos 3 cambios ya deberíamos notar una mejora grande en la próxima sesión real.
