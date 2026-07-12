# Plan UX para recuperación de turnos interrumpidos u ocupados

## Estado

Propuesto.

## Objetivo

Mejorar la UX del chat cuando una respuesta del agente:

- se corta,
- queda visualmente incompleta,
- sigue procesándose en backend aunque la UI parezca idle,
- o recibe un nuevo mensaje del usuario mientras el turno anterior sigue vivo.

La meta es que el usuario **nunca tenga que adivinar** si el sistema:

- sigue procesando,
- quedó interrumpido,
- puede retomarse,
- o dejó su mensaje en cola.

---

## Problema observado

En la prueba manual apareció este patrón:

1. la respuesta quedó incompleta o inconsistente;
2. la UI no mostró botón `Continuar`;
3. la UI tampoco mostró una señal visible de `processing`;
4. el usuario escribió manualmente `continua`;
5. el runtime devolvió un error tipo:
   - `Agent is already processing...`

Esto revela un descalce entre:

- **estado visual percibido por el usuario**, y
- **estado real del runtime / turno**.

Resumen:

> la UX no está guiando al usuario; lo obliga a improvisar.

---

## Principios UX

1. **Nunca dejar el chat en silencio ambiguo.**
2. **La UI debe expresar el estado del turno, no ocultarlo.**
3. **La intención de retomar un turno debe resolverse principalmente por estado, no por keyword.**
4. **Si el runtime sigue ocupado, el sistema debe encolar o recuperar, no rechazar de inmediato.**
5. **Los errores internos del runtime no deben escaparse crudos a la UI.**
6. **El input del chat debe volverse recovery-aware cuando exista un turno activo, interrumpido o resumible.**

---

## Contexto de integración

### Proyecto consumidor

- Repo local: `C:\_git\einarc\testboi1`
- UI del chat: `pkg/agent/ui/page.templ`
- Prompt HTTP: `pkg/agent/http/prompt.go`
- Orquestación del turno: `pkg/agent/application/manager.go`

### Proyecto proveedor / gateway

- Repo local: `C:\_git\einarc\sync-ai-gateway`
- Endpoint upstream: `POST /api/gateway/v1/chat/completions`

Este plan se enfoca primero en el **consumidor**, porque el problema visible en la
prueba es principalmente de señalización y manejo UX del turno.

---

## Estado UX objetivo

El usuario debe poder distinguir al menos estos estados:

- `Procesando`
- `Respuesta interrumpida`
- `Recuperando respuesta`
- `Retomando respuesta`
- `Mensaje en cola`
- `Completado`
- `Falló`

Y debe tener acciones claras según el caso:

- `Retomar respuesta`
- `Enviar como nuevo mensaje`
- `Reintentar`
- `Esperar / mensaje en cola`

---

## Modelo UX deseado: state-driven, no keyword-driven

La UX no debe depender principalmente de que el usuario escriba una palabra
exacta como `continua`.

El enfoque correcto es este:

```text
estado del turno
  ↓
la UI entra en modo recovery-aware
  ↓
el usuario puede:
  - retomar
  - esperar
  - enviar nuevo mensaje
  ↓
recién al final, si hace falta, una heurística textual ayuda
```

## Regla

- **camino principal:** estado + acciones explícitas
- **camino secundario:** heurística textual como fallback de cortesía

Esto evita adaptar el producto a una forma particular de escribir.

---

## Fase 1 — Señalización visible del estado del turno

## Objetivo

Eliminar el estado silencioso donde la UI parece idle pero el runtime sigue vivo
u ocupado.

## Cambios

1. Mostrar un banner visible cuando el runtime siga ocupado:
   - `El agente sigue procesando…`
   - `Recuperando respuesta…`
   - `La respuesta se interrumpió…`
2. No mostrar visualmente `idle` si el sistema todavía no puede aceptar un turno
   nuevo con seguridad.
3. Si hay texto parcial del assistant sin cierre confiable:
   - badge `Respuesta interrumpida`
   - CTA `Retomar respuesta`

## Archivos

- `pkg/agent/ui/page.templ`
- potencialmente `pkg/agent/application/manager.go` si hace falta exponer mejor
  el estado real del turno

## Criterio de aceptación

- no existe el caso "pantalla muda" sin banner, spinner o CTA;
- si el runtime sigue ocupado, la UI lo refleja;
- si la respuesta quedó parcial, el usuario lo ve.

---

## Fase 2 — Input recovery-aware

## Objetivo

Hacer que el input del chat cambie de comportamiento cuando existe un turno
recuperable, resumible o todavía ocupado.

## Cambios

1. Cuando exista un turno en estado:
   - activo,
   - interrumpido,
   - recuperable,
   - o resumible,
   la UI entra en modo **recovery-aware**.
2. En ese modo, el submit del usuario no se trata ciegamente como prompt normal.
3. La UI debe ofrecer o inferir una decisión entre:
   - `Retomar respuesta`
   - `Enviar como nuevo mensaje`
   - `Esperar / dejar en cola`
4. Si el usuario intenta escribir mientras el turno sigue vivo, la UI debe guiar
   la acción en vez de delegar la interpretación al modelo.

## Archivos

- `pkg/agent/ui/page.templ`
- `pkg/agent/application/manager.go`

## Criterio de aceptación

- la UX ya no depende del botón únicamente;
- la UX ya no depende de una palabra exacta;
- el input se comporta distinto cuando hay un turno recuperable.

---

## Fase 3 — Cola mínima de mensajes mientras el turno sigue ocupado

## Objetivo

Evitar que el usuario reciba errores crudos tipo `already processing` cuando el
runtime todavía está trabajando.

## Estrategia mínima v1

Implementar una cola de **1 mensaje pendiente por sesión**.

## Cambios

1. Si llega un mensaje mientras el turno anterior sigue vivo:
   - no rechazar de inmediato;
   - guardarlo como pendiente.
2. Si entra otro mensaje mientras ya existe uno pendiente:
   - reemplazar el anterior (versión lazy v1).
3. Si la intención era retomar:
   - guardar `pendingAction = resume`
   - no guardarlo como prompt libre ambiguo.
4. Mostrar un banner visible:
   - `Mensaje en cola`
   - `Retomaremos la respuesta al terminar el turno actual`

## Archivos

- `pkg/agent/application/manager.go`
- `pkg/agent/ui/page.templ`

## Criterio de aceptación

- el usuario no ve el error crudo `Agent is already processing...`;
- el sistema conserva la intención del usuario;
- al terminar el turno actual, se procesa la acción pendiente.

---

## Fase 4 — Diferenciar recover, replay y continuation en la UX

## Objetivo

Hacer visible qué está haciendo realmente el sistema.

## Cambios

1. Si el problema fue solo transporte / stream:
   - mostrar `Recuperando respuesta…`
2. Si la respuesta ya existe y se está reproduciendo:
   - mostrar `Reproduciendo respuesta…`
3. Si hace falta pedir más generación al runtime:
   - mostrar `Retomando respuesta…`
4. Si el usuario decidió empezar de nuevo:
   - mostrar `Reintentando…`

## Archivos

- `pkg/agent/ui/page.templ`
- `pkg/agent/application/manager.go`

## Criterio de aceptación

- el usuario distingue entre recuperar lo ya generado y seguir generando;
- la UI deja de usar un único estado genérico para casos distintos.

---

## Fase 5 — Heurística textual como fallback, no como diseño principal

## Objetivo

Aceptar comportamientos naturales del usuario sin adaptar el producto a una sola
palabra o estilo personal.

## Cambios

1. Mantener una heurística textual mínima solo como última capa de cortesía.
2. No basarse únicamente en `continua` / `sigue` / `...`.
3. Si existe un turno resumible y el mensaje del usuario es corto, ambiguo y no
   agrega contenido nuevo, el sistema puede interpretarlo como intención de
   recovery.
4. Si el contexto no alcanza para inferirlo con seguridad:
   - preguntar o tratarlo como mensaje nuevo, según la UX elegida.

## Regla

La heurística textual:

- **sí ayuda**,
- pero **no define el diseño principal del sistema**.

## Archivos

- `pkg/agent/ui/page.templ`
- `pkg/agent/http/prompt.go`
- `pkg/agent/application/manager.go`

## Criterio de aceptación

- el sistema no depende de una keyword exacta;
- el fallback textual mejora UX, pero no gobierna la arquitectura.

---

## Fase 6 — Reglas explícitas de decisión UX

## Objetivo

Que el comportamiento sea predecible y que la UI no exponga detalles internos
innecesarios.

## Reglas propuestas

1. Si el turno sigue activo:
   - no mandar prompt normal;
   - recover o queue.
2. Si el turno está interrumpido y es resumible:
   - ofrecer `Retomar respuesta`.
3. Si no hay contexto suficiente para retomar:
   - mostrar error claro:
     - `No pude retomar la respuesta anterior.`
4. Si el runtime respondió busy pero la UI estaba idle:
   - corregir el estado visual de inmediato.
5. Si el usuario escribe mientras existe un turno recuperable:
   - decidir primero por estado del turno;
   - no por keyword textual aislada.

## Criterio de aceptación

- no se muestran errores internos crudos al usuario;
- la UI corrige su estado cuando descubre desalineación con el runtime;
- la decisión principal depende del estado, no de una palabra exacta.

---

## Matriz de responsabilidad

| Problema | Repo | Archivo | Cambio mínimo |
|---|---|---|---|
| UI parece idle mientras el runtime sigue ocupado | `testboi1` | `pkg/agent/ui/page.templ` | banner visible + estado real del turno |
| el input no cambia de modo ante un turno recuperable | `testboi1` | `pkg/agent/ui/page.templ` | modo recovery-aware |
| error `already processing` llega crudo al usuario | `testboi1` | `manager.go` / `page.templ` | cola mínima o recover automático |
| falta CTA cuando la respuesta queda parcial | `testboi1` | `page.templ` | badge + botón `Retomar respuesta` |
| la UI no diferencia recovery vs continuation | `testboi1` | `page.templ` | copy y estado semántico |
| fallback textual domina demasiado el diseño | `testboi1` | `page.templ` / `prompt.go` / `manager.go` | bajar keywords a fallback secundario |

---

## Plan de validación manual

## Escenario 1 — Respuesta interrumpida con señal visible

1. provocar corte visual o inconsistencia;
2. verificar que aparece banner o badge;
3. verificar que no queda la UI muda.

## Escenario 2 — Input recovery-aware

1. provocar una respuesta resumible;
2. intentar escribir un mensaje nuevo;
3. verificar que la UI no lo trate automáticamente como prompt normal;
4. verificar que ofrece retomar, encolar o enviar como nuevo.

## Escenario 3 — Turno aún ocupado

1. enviar un mensaje mientras el runtime sigue procesando;
2. verificar que no aparece error crudo;
3. verificar que el mensaje queda en cola o se muestra recuperación.

## Escenario 4 — CTA visible

1. provocar una respuesta parcial;
2. verificar que aparece `Retomar respuesta`.

## Escenario 5 — Distinción visual

1. forzar un caso de recover de stream;
2. forzar un caso de continuation real;
3. verificar que la UI no usa exactamente el mismo copy para ambos.

## Escenario 6 — Heurística textual secundaria

1. provocar un turno resumible;
2. escribir una frase corta ambigua;
3. verificar que la UX no depende exclusivamente de una keyword fija.

---

## Prioridad sugerida

1. **Fase 1** — señalización visible
2. **Fase 2** — input recovery-aware
3. **Fase 3** — cola mínima
4. **Fase 4** — copy semántico
5. **Fase 5** — heurística textual secundaria
6. **Fase 6** — endurecer reglas de decisión

---

## Resumen ejecutivo

La UX actual falla porque el usuario puede quedar en este estado:

- la respuesta se ve incompleta;
- no hay botón;
- no hay señal de processing;
- escribe algo para retomar;
- recibe un error interno del runtime.

La mejora correcta es:

- mostrar el estado del turno,
- volver el input **recovery-aware**,
- encolar mensajes si el turno sigue ocupado,
- decidir principalmente por **estado** y no por keywords,
- y dejar la heurística textual solo como fallback de cortesía.
