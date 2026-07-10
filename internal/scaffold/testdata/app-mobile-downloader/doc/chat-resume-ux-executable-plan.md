# Plan ejecutable: reanudación de respuestas truncadas

## Estado

Propuesto.

## Objetivo

Corregir el caso en que una respuesta del agente se corta y luego `continua`
se interpreta como un turno nuevo ambiguo en vez de una reanudación real.

Objetivo práctico:

1. el usuario ve claramente que la respuesta quedó interrumpida;
2. el sistema decide correctamente entre **replay** y **continuation**;
3. la continuación mantiene el hilo sin repetir de más.

---

## Contexto de integración

### Proyecto proveedor de chat completions

- Repo local: `C:\_git\einarc\sync-ai-gateway`
- Endpoint confirmado: `POST /api/gateway/v1/chat/completions`
- Archivo relevante: `internal/gateway/http/chat_completions.go`

### Proyecto consumidor

- Repo local: `C:\_git\einarc\scaffoldxd1`
- Entrada HTTP actual del chat: `POST /agent/sessions/{id}/prompt`
- UI del chat: `pkg/agent/ui/page.templ`
- Handler HTTP del prompt: `pkg/agent/http/prompt.go`
- Orquestación de runtime/sesión: `pkg/agent/application/manager.go`

---

## Hallazgos confirmados en código

## Ya existe en el consumidor

En `pkg/agent/ui/page.templ` ya existen señales útiles:

- `turnIsOpen`
- `turnPhase`
- `turnHasVisibleAssistantText`
- `pendingTerminalError`
- `lastEventIds`
- botones `Continuar` y `Reintentar`
- reconexión de stream
- sellado desde `/history`

En `pkg/agent/http/prompt.go` y `pkg/agent/application/manager.go`:

- el endpoint de prompt acepta solo `message`;
- `sendPrompt()` reenvía texto crudo;
- `Manager.Prompt(ctx, id, message)` recibe solo string;
- no existe payload estructurado de resume;
- no existe decisión explícita entre replay y continuation.

## Gap real

La UI ya tiene base para mostrar un estado interrumpido, pero el backend aún no
sabe distinguir entre:

- nuevo prompt;
- replay de una respuesta ya generada;
- continuation real contra runtime/modelo.

---

## Principios de diseño

1. **El frontend no debe conocer detalles internos** como `resumeFromSeq`.
2. **La UI expresa intención; el backend decide estrategia.**
3. **`action: "resume"` no siempre implica llamar al modelo.** Primero hay que
   decidir si basta con recuperar lo ya persistido.
4. **El estado canónico del turno pertenece al dominio/application**, no a la UI,
   aunque en v1 la UI pueda seguir representando parte del estado local.
5. **Mantener un solo endpoint en v1** está bien; separar recursos puede venir
   después si hace falta.

---

## Modelo funcional

### Intención del usuario

`resume` significa:

> **quiero retomar este turno**.

No significa automáticamente:

- "manda `continua` al modelo", ni
- "reconecta SSE y cruza los dedos".

### Operación interna del sistema

Internamente el sistema debería razonar esto como recuperación del turno:

```text
Turn.Recover()
  ├─ Replay
  └─ Continuation
```

La UI pide `resume`. El backend decide si eso se resuelve con:

- **replay**: recuperar una respuesta ya generada;
- **continuation**: generar texto nuevo porque realmente faltó respuesta.

### Algoritmo objetivo

```text
UI pide resume
   ↓
host inspecciona el turno
   ↓
¿la respuesta completa ya existe y solo faltó entregarla?
   ├─ sí  → replay
   └─ no  → continuation vía runtime/modelo
```

---

## Máquina mínima de estados del turno

Formalizar al menos estos estados:

- `STREAMING`
- `INTERRUPTED`
- `REPLAYING`
- `RESUMING`
- `COMPLETED`
- `FAILED`

## Regla

El código puede seguir usando variables simples en v1, pero el plan de
implementación debe mapearlas a estos estados explícitos para evitar
transiciones ambiguas.

## Nota arquitectónica

La UI puede seguir teniendo estado derivado local, pero el objetivo es que el
backend publique un estado formal del turno y la UI solo lo represente.

---

## Contrato mínimo ejecutable

## Payload propuesto v1

```json
{
  "message": "continua",
  "action": "resume"
}
```

### Variante opcional si hace falta anclar al turno

```json
{
  "message": "continua",
  "action": "resume",
  "turnId": "turn-123"
}
```

## Regla

- `resumeFromSeq` no forma parte del contrato frontend.
- El frontend expresa **intención** (`action: "resume"`), no estrategia.
- Si el backend necesita `seq`, `tail`, offsets o event ids, los resuelve solo.

## Compatibilidad

- si `action` no viene, el comportamiento actual se mantiene;
- si `action=resume`, el backend no trata el mensaje como prompt normal.

---

## Finish reason

La decisión entre replay y continuation mejora mucho si el sistema persiste o
puede derivar una razón explícita de finalización del turno.

## Modelo inicial sugerido

```go
type TurnFinishReason string
```

Valores iniciales sugeridos:

- `completed`
- `interrupted`
- `timeout`
- `client_disconnected`
- `provider_closed`

## Regla

No hace falta resolver todos los productores en v1, pero el modelo debe quedar
previsto para depender menos de heurísticas a futuro.

---

## Alcance

Este plan cubre:

1. cambios UX en `scaffoldxd1`;
2. cambios HTTP mínimos en `scaffoldxd1`;
3. lógica explícita de recover → replay o continuation en `scaffoldxd1`;
4. validación puntual en `sync-ai-gateway`.

## No cubre en v1

- rediseño completo del protocolo de eventos;
- endpoint nuevo `/turns/{id}/resume`;
- provider-specific resume sofisticado;
- auto-reanudación silenciosa sin señal al usuario;
- múltiples intentos de continuation sobre el mismo turno.

### Nota V2

Más adelante conviene decidir cómo modelar varios intentos de continuation sobre
el mismo turno para auditoría, métricas y depuración.

---

## Fases ejecutables

# Fase 1 — Consumidor: contrato de resume semántico

## Objetivo

Agregar soporte backend para distinguir resume de prompt normal sin exponer
internals al frontend.

## Archivos

- `pkg/agent/http/prompt.go`
- archivo donde esté definido `messageRequest` si vive aparte
- tests del handler

## Cambios

1. Extender `messageRequest` con:
   - `Action string \`json:"action"\``
   - opcional: `TurnID string \`json:"turnId"\``
2. En `sendPrompt()`, pasar estos datos a la capa application.
3. Mantener backward compatibility con payload viejo.
4. Mapear internamente:
   - `action="resume"` → intención de retomar turno;
   - ausencia de `action` → prompt normal.

## Decisión de implementación

No crear endpoint nuevo. Reusar `POST /agent/sessions/{id}/prompt`.

## Criterio de aceptación

- el endpoint acepta payload viejo y nuevo;
- `action=resume` llega a la capa application;
- un prompt normal sigue funcionando sin cambios.

---

# Fase 2 — Consumidor: formalizar estado del turno

## Objetivo

Dejar explícito cuándo un turno está interrumpido, reejugándose o reanudándose.

## Archivos

- `pkg/agent/application/manager.go`
- `pkg/agent/ui/page.templ`
- estructuras/metadata de sesión o history si hace falta soporte adicional

## Cambios

1. Mapear el estado actual a los estados formales:
   - `STREAMING`
   - `INTERRUPTED`
   - `REPLAYING`
   - `RESUMING`
   - `COMPLETED`
   - `FAILED`
2. Asegurar que `Continuar` solo aparezca cuando el turno sea resumible.
3. Diferenciar visualmente:
   - respuesta interrumpida;
   - recuperación de stream;
   - continuación real.
4. Dejar explícito qué parte del estado vive solo en UI como compatibilidad v1
   y qué parte debe migrar a application.

## Criterio de aceptación

- no hay confusión entre turno en curso y turno interrumpido;
- la UI muestra claramente si está recuperando o reanudando;
- el plan deja claro que el estado objetivo vive en backend/dominio.

---

# Fase 3 — Consumidor: decisión interna recover → replay o continuation

## Objetivo

Evitar llamar al modelo cuando la respuesta completa ya existe.

## Archivos

- `pkg/agent/application/manager.go`
- `history.go`, `event.go` o donde convenga leer transcript/eventos persistidos
- tests de manager

## Cambios

1. Agregar una ruta interna tipo `promptWithOptions(ctx, id, message, opts)`.
2. Definir `PromptOptions` con:
   - `Action string`
   - opcional `TurnID string`
3. Si `Action=="resume"`, el manager debe decidir:
   - **replay**: si la respuesta ya existe y solo faltó entregarla;
   - **continuation**: si la respuesta realmente quedó incompleta.
4. Para esa decisión, usar señales ya disponibles o fáciles de agregar:
   - transcript persistido;
   - `lastEventIds` / history;
   - cierre limpio vs cierre perdido;
   - texto final sellado desde `/history`;
   - `finish_reason` cuando exista.

## Regla

No mandar `continua` crudo al runtime si antes no se descartó replay.

## Criterio de aceptación

- `action=resume` primero inspecciona el turno existente;
- si hay contenido ya generado, se recupera sin tokens nuevos;
- solo se llama al runtime cuando realmente falta generación.

---

# Fase 4 — Consumidor: continuation real contra runtime

## Objetivo

Cuando replay no alcanza, continuar la respuesta sin perder hilo.

## Archivos

- `pkg/agent/application/manager.go`
- tests de manager

## Cambios

1. Si el manager decide `continuation`, reconstruir contexto mínimo útil:
   - último assistant truncado;
   - tail del texto visible;
   - estado del turno.
2. Reescribir la instrucción al runtime antes de `runtime.Prompt(...)`.

## Rewrite sugerido

```text
Tu respuesta anterior fue interrumpida.

Continúa exactamente desde este punto, sin repetir ni reformular lo ya dicho:
"...tail del mensaje..."

No empieces de nuevo. No agregues introducción. Sigue el mismo párrafo o lista.
```

## Regla

Esto es un fallback de continuation, no el camino por defecto de todo resume.

## Criterio de aceptación

- el runtime solo recibe continuation cuando replay no aplica;
- la continuación mantiene el hilo;
- si falta contexto suficiente, el backend falla claro.

---

# Fase 5 — Consumidor: UX visible de interrupción y reanudación

## Objetivo

Hacer visible al usuario qué está pasando y evitar que tenga que adivinar.

## Archivo

- `pkg/agent/ui/page.templ`

## Cambios

1. Marcar el último mensaje assistant como `interrupted` cuando:
   - hubo texto visible;
   - no hubo cierre limpio;
   - el turno quedó resumible.
2. Mostrar copy claro:
   - `La respuesta se interrumpió antes de terminar.`
3. Hacer que el botón `Continuar`:
   - primero intente `action="resume"`;
   - deje al backend decidir replay o continuation.
4. Mantener `Reintentar` para casos donde no hubo texto útil visible.
5. Anexar visualmente la recuperación/continuación al mismo hilo de respuesta.

## Criterio de aceptación

- si hubo texto parcial, aparece CTA correcta;
- `Continuar` ya no significa solo reconectar SSE;
- la UX deja claro si se recuperó o si se siguió generando.

---

# Fase 6 — Consumidor: fallback para `continua` manual

## Objetivo

Cubrir al usuario que escribe `continua` en vez de usar el botón.

## Archivos

- `pkg/agent/http/prompt.go`
- `pkg/agent/application/manager.go`

## Cambios

1. Si el mensaje del usuario es exactamente uno de estos:
   - `continua`
   - `continuar`
   - `sigue`
   - `...`
2. y existe un turno resumible,
3. convertirlo internamente a `action="resume"`.

## Regla

Esto es fallback. El camino principal sigue siendo la acción estructurada.

## Criterio de aceptación

- `continua` manual funciona cuando hay turno truncado;
- `continua` normal sigue siendo texto normal cuando no hay nada que reanudar.

---

# Fase 7 — Gateway: verificar origen del corte inicial

## Objetivo

Determinar si el corte original viene del gateway, del runtime o del lado SSE.

## Repo

- `C:\_git\einarc\sync-ai-gateway`

## Archivos a revisar / tocar

- `internal/gateway/http/chat_completions.go`
- `internal/gateway/application/stream_usage.go`
- cliente upstream activo según modelo
- scripts de prueba como `scripts/test-gateway.sh`

## Cambios / validaciones

1. Verificar que el stream cierre con señal consistente.
2. Verificar logs de timeout o cierre prematuro.
3. Confirmar si el upstream terminó limpio o no.
4. Confirmar si el host recibe fin de stream aunque falte cierre semántico.
5. Agregar logging mínimo si hoy no alcanza.

## Criterio de aceptación

- se puede clasificar el origen del corte como:
  - gateway,
  - upstream model,
  - runtime host,
  - SSE/browser.

---

## Matriz de responsabilidades

| Responsabilidad | Repo | Archivo | Cambio mínimo |
|---|---|---|---|
| Soportar payload de resume semántico | `scaffoldxd1` | `pkg/agent/http/prompt.go` | aceptar `action` y opcional `turnId` |
| Formalizar estado de turno | `scaffoldxd1` | `pkg/agent/application/manager.go` + `pkg/agent/ui/page.templ` | backend como estado canónico, UI como representación |
| Decidir recover → replay o continuation | `scaffoldxd1` | `pkg/agent/application/manager.go` | inspección del turno antes de llamar runtime |
| Continuation real cuando falta generación | `scaffoldxd1` | `pkg/agent/application/manager.go` | rewrite del prompt como fallback |
| Introducir `finish_reason` inicial | `scaffoldxd1` | `application` / metadata de turno | simplificar decisión y observabilidad |
| Mostrar estado interrumpido y CTA | `scaffoldxd1` | `pkg/agent/ui/page.templ` | banner/copy + botón que mande `action="resume"` |
| Fallback para `continua` manual | `scaffoldxd1` | `prompt.go` / `manager.go` | mapear a resume cuando aplique |
| Aislar origen del corte inicial | `sync-ai-gateway` | `internal/gateway/http/chat_completions.go` y related | validar cierre de stream, timeout y logs |

---

## Plan de validación

## Escenario 1 — respuesta normal

1. enviar prompt corto;
2. recibir cierre limpio;
3. confirmar que no aparece CTA de resume.

## Escenario 2 — stream cortado pero respuesta completa persistida

1. forzar pérdida del SSE después de que el backend ya tenga texto suficiente;
2. ejecutar `Continuar`;
3. verificar que ocurre **replay**, no llamada nueva al modelo.

## Escenario 3 — respuesta realmente incompleta

1. forzar corte temprano real;
2. ejecutar `Continuar`;
3. verificar que ocurre **continuation** contra runtime;
4. verificar que no repite grandes bloques.

## Escenario 4 — `continua` manual

1. dejar un turno resumible;
2. escribir `continua`;
3. verificar que el sistema lo convierte en resume.

## Escenario 5 — `continua` sin turno resumible

1. conversación normal cerrada;
2. escribir `continua`;
3. verificar que se trata como mensaje normal.

## Escenario 6 — resume sin contexto suficiente

1. simular `action="resume"` sin turno recuperable ni continuable;
2. verificar error claro del backend.

## Escenario 7 — corte aislado por gateway

1. correr prueba controlada contra `sync-ai-gateway`;
2. revisar logs y cierre de stream;
3. clasificar origen del corte.

---

## Criterios de aceptación globales

## UX

- el usuario entiende cuándo una respuesta quedó incompleta;
- puede continuarla con un click;
- la UI deja claro si se recuperó o si siguió generando.

## Backend

- existe payload explícito de resume;
- el frontend no depende de `seq` internos;
- `action=resume` decide entre replay y continuation;
- `continua` ya no depende de interpretación ambigua del modelo.

## Observabilidad

- se puede distinguir entre corte de stream y mala lógica de resume.
- el modelo de `finish_reason` queda previsto para simplificar diagnóstico.

---

## Orden de ejecución sugerido

1. Fase 1
2. Fase 2
3. Fase 3
4. Fase 4
5. Fase 5
6. Fase 6
7. Fase 7

---

## Entregables

1. Cambio de contrato HTTP para soportar `action="resume"`.
2. Estado formal de turno para casos interrumpidos.
3. Lógica de manager para decidir recover → replay o continuation.
4. Continuation real solo cuando haga falta.
5. Modelo inicial de `finish_reason`.
6. UI con estado `interrupted` y CTA útil.
7. Validación del gateway para aislar la causa del corte inicial.

---

## Resumen ejecutivo

La mejora correcta no es solo "mandar `continua` mejor".

La mejora correcta es:

- modelar `action: "resume"` explícitamente;
- no exponer internals como `resumeFromSeq` al frontend;
- decidir primero entre **replay** y **continuation**;
- formalizar estado del turno en el dominio y no solo en la UI;
- dejar previsto `finish_reason` para simplificar la decisión;
- después investigar si el corte inicial viene del gateway.
