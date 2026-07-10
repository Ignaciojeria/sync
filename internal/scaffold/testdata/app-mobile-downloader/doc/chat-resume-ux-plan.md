# Plan UX para respuestas truncadas y reanudación del chat

## Objetivo

Evitar que el usuario pierda el hilo cuando una respuesta del agente se corta a
mitad y hacer que la acción de continuar funcione como una **reanudación real**,
no como un turno nuevo ambiguo.

Este plan asume que el proyecto consume un endpoint tipo **chat completions** a
través de `sync-ai-gateway`, pero se enfoca primero en la UX y en el contrato
entre frontend y backend del host.

---

## Contexto de integración

### Proyecto proveedor de chat completions

- Repo local: `C:\_git\einarc\sync-ai-gateway`
- Rol: expone el endpoint upstream de **chat completions** consumido por este
  proyecto.
- Endpoint a verificar en la implementación real: `/v1/chat/completions`
  (o la ruta efectiva configurada en el gateway si difiere).

### Proyecto consumidor

- Repo local: `C:\_git\einarc\scaffoldxd1`
- Rol: host del chat del agente que consume el upstream vía runtime / gateway.
- Entrada HTTP del chat en este proyecto: `/agent/sessions/{id}/prompt`
- UI del chat: `pkg/agent/ui/page.templ`
- Orquestación HTTP del prompt/resume: `pkg/agent/http/prompt.go`
- Lógica de sesión y runtime: `pkg/agent/application/manager.go`

### Criterio de reparto de responsabilidades

- **Consumidor (`scaffoldxd1`)**:
  - detectar turno interrumpido;
  - mostrar CTA de continuación;
  - soportar `resume=true` o acción equivalente;
  - reconstruir la intención de reanudación.
- **Proveedor (`sync-ai-gateway`)**:
  - ayudar a confirmar si el corte inicial viene de timeout, stream,
    cierre prematuro o límite de output;
  - exponer señales suficientes para distinguir cierre limpio de interrupción.

### Objetivo de esta mejora

La mejora de UX de reanudación se implementa primero en el consumidor.
La investigación del corte inicial se valida después en el proveedor
`sync-ai-gateway`.

---

## Problema observado

En la interacción revisada aparecieron dos fallas distintas:

1. **Respuesta truncada**
   - el texto quedó cortado a mitad de frase o palabra;
   - la UI no dejó suficientemente claro si el turno terminó o falló.

2. **Continuación incorrecta**
   - el usuario escribió `continua`;
   - el sistema trató ese texto como prompt normal;
   - el agente respondió otra cosa o repitió ideas, en vez de seguir desde el
     punto exacto donde se cortó.

Resumen:

> El corte inicial huele a runtime / gateway / stream. La mala continuación es
> claramente un problema de UX + contrato de orquestación.

---

## Principio UX

El usuario no debería tener que diagnosticar si:

- el stream sigue vivo,
- el turno cerró limpio,
- la respuesta quedó incompleta,
- o `continua` será interpretado como resume o como nuevo mensaje.

La UX debe modelar explícitamente estos estados:

- `generando`
- `completado`
- `interrumpido`
- `abortado`
- `reanudando`

---

## Alcance

Este plan cubre:

1. **Frontend del chat** (`pkg/agent/ui/page.templ`)
2. **Contrato HTTP del host** (`/agent/sessions/{id}/prompt` o equivalente)
3. **Lógica de resume en el host / manager**
4. **Instrumentación mínima para diferenciar corte real vs cierre limpio**

No propone, en v1:

- rediseñar `sync-ai-gateway`,
- cambiar el proveedor del modelo,
- rehacer el protocolo de eventos completo.

---

## Resultado esperado

Cuando una respuesta se corta:

1. el usuario ve que quedó **interrumpida**;
2. aparece una acción explícita **Continuar respuesta**;
3. al continuar, el sistema **reanuda** desde el último fragmento útil;
4. la continuación se percibe como parte de la misma respuesta, no como una
   nueva respuesta desconectada.

---

## Propuesta UX mínima viable

## 1. Estado visible del último turno

Cada mensaje del asistente debe tener estado explícito:

- `completed`
- `interrupted`
- `aborted`

### Regla

Si el stream terminó sin cierre limpio, la UI **no** debe mostrar la respuesta
como si hubiera finalizado correctamente.

### Señales visuales sugeridas

- badge `Respuesta interrumpida`
- botón `Continuar respuesta`
- copy `La respuesta se interrumpió antes de terminar.`

---

## 2. Acción explícita de reanudar

No depender del texto libre `continua`.

En vez de eso, la UI debe disparar una acción estructurada de resume.

### Payload sugerido

```json
{
  "message": "continua",
  "resume": true,
  "resumeFromSeq": 438
}
```

Mejor todavía, si el contrato se puede ajustar:

```json
{
  "action": "resume",
  "resumeFromSeq": 438
}
```

### Regla backend

Si `resume=true` o `action=resume`:

- no tratarlo como prompt nuevo;
- reconstruir el contexto del último mensaje assistant incompleto;
- pedir al runtime que continúe **exactamente** desde ese punto;
- evitar repetir bloques ya visibles.

---

## 3. Marcado visual de respuesta incompleta

Cuando el último mensaje quedó truncado:

- marcarlo como incompleto;
- no dejarlo visualmente igual que un mensaje final;
- ofrecer CTA inmediata.

### Objetivo

Que el usuario entienda:

- "esto no terminó" y
- "puedo seguirlo con un click".

---

## 4. Continuación unida al mismo mensaje

La continuación debería anexarse al mismo bloque visual o quedar claramente
agrupada como continuación de la respuesta anterior.

### Evitar

- una nueva respuesta sin relación visual;
- duplicación del encabezado del asistente;
- sensación de "el agente cambió de tema".

---

## Detección de truncado

## Fuente ideal de verdad

No usar solo puntuación final. La fuente de verdad debería ser el **estado del
turno** y del **stream**.

## Señales para marcar `interrupted`

- se cerró el stream sin evento final esperado;
- el turno pasó a idle sin `message_end` / `turn_end` consistente;
- hubo timeout o error de red durante generación;
- el texto termina a mitad de palabra o frase;
- hubo contenido visible pero no cierre semántico del turno.

## Heurística mínima de respaldo

Si no existe señal estructurada suficiente, usar una heurística simple:

- hubo texto visible del assistant,
- el stream se perdió,
- y el mensaje no parece haber terminado limpiamente.

Eso basta para habilitar `interrupted + resumable`.

---

## Contrato recomendado por mensaje

Agregar o derivar estos campos por item del historial:

```json
{
  "seq": 438,
  "kind": "assistant",
  "text": "...",
  "status": "completed | interrupted | aborted",
  "resumable": true
}
```

### Beneficio

La UI deja de adivinar y pasa a renderizar un estado explícito.

---

## Lógica de resume en el host

## Regla funcional

Si el usuario pide continuar y el último mensaje assistant está truncado:

- tomar el `seq` del último assistant incompleto;
- recuperar un tail del texto ya emitido;
- mandar al runtime una instrucción de continuación, no un mensaje libre
  ambiguo.

### Prompt rewrite sugerido

```text
Tu respuesta anterior fue interrumpida.

Continúa exactamente desde este punto, sin repetir ni reformular lo ya dicho:
"...¿Cuántos patches se acum"

No empieces de nuevo. No agregues introducción. Sigue el mismo párrafo o lista.
```

### Objetivo

Resolver el caso `continua` aunque el usuario no sea preciso.

---

## Casos de uso a cubrir

## Caso 1: respuesta normal

- llega cierre limpio;
- estado `completed`;
- no se muestra CTA de resume.

## Caso 2: respuesta cortada con texto visible

- estado `interrupted`;
- mensaje marcado como incompleto;
- se muestra `Continuar respuesta`.

## Caso 3: usuario escribe un mensaje nuevo cuando hay una respuesta interrumpida

Opciones válidas:

- interpretar el nuevo mensaje como nuevo turno;
- o preguntar si desea continuar la respuesta anterior.

Para v1, mantenerlo simple:

- si el usuario escribe otro mensaje distinto de `continua` / `sigue`, tratarlo
  como turno nuevo.

## Caso 4: usuario escribe `continua`

- si hay último assistant truncado: ejecutar resume;
- si no lo hay: tratarlo como mensaje normal.

---

## Plan por fases

## Fase 1 — Quick wins en el consumidor

### Objetivo

Eliminar la mayor parte de la confusión sin tocar el gateway.

### Cambios

1. detectar `interrupted` en el último turno;
2. mostrar badge + CTA `Continuar respuesta`;
3. enviar `resume=true` al backend;
4. guardar `resumeFromSeq`;
5. anexar visualmente la continuación al mismo mensaje.

### Valor

Alto.

### Costo

Bajo.

---

## Fase 2 — Lógica explícita de resume en backend

### Objetivo

Dejar de tratar `continua` como prompt ambiguo.

### Cambios

1. aceptar un payload estructurado de resume;
2. reconstruir contexto desde el último assistant incompleto;
3. reescribir la instrucción al runtime para continuar exacto;
4. evitar repetición de bloques visibles.

### Valor

Muy alto.

### Costo

Bajo a medio.

---

## Fase 3 — Instrumentación del stream

### Objetivo

Separar claramente problemas de UX de problemas de runtime/gateway.

### Cambios

1. registrar inicio y fin real del turno;
2. distinguir `completed` vs `interrupted` vs `aborted`;
3. detectar ausencia de evento final esperado;
4. medir cortes a mitad de respuesta.

### Valor

Alto.

### Costo

Medio.

---

## Fase 4 — Investigación específica del gateway

### Objetivo

Verificar si el corte inicial viene de `sync-ai-gateway`, del runtime o del
cliente SSE.

### Preguntas a responder

- ¿falta un evento de cierre?
- ¿hay timeout de upstream?
- ¿se corta el SSE del browser?
- ¿el host pasa a `idle` antes del final real?
- ¿el modelo agotó output tokens?

### Nota

Esta fase no bloquea la Fase 1. El resume debe existir aunque el gateway quede
perfecto.

---

## Métricas recomendadas

Medir esto, no impresiones subjetivas:

- % de respuestas interrumpidas
- % de respuestas interrumpidas recuperadas con resume
- % de resumes que repiten contenido ya visible
- % de usuarios que escriben `continua`, `sigue`, `ping`, `?`
- tiempo promedio de recuperación tras corte
- tasa de abandono tras respuesta interrumpida

---

## Criterios de aceptación

## UX

- si una respuesta se corta, el usuario lo ve con claridad;
- existe una acción visible para continuar;
- la continuación sigue el hilo y no reabre el tema desde cero.

## Backend

- el host distingue `completed` de `interrupted`;
- existe un payload explícito de resume;
- `continua` con turno truncado no se trata como prompt normal.

## Observabilidad

- es posible saber si el problema fue corte de stream o mala lógica de resume.

---

## Recomendación concreta

Orden de implementación recomendado:

1. **Consumidor/UI**: detectar `interrupted` + mostrar `Continuar respuesta`.
2. **Host/backend**: soportar `resume=true` + `resumeFromSeq`.
3. **Resume real**: reescribir instrucción al runtime con tail del mensaje.
4. **Instrumentación**: medir dónde se corta realmente el stream.
5. **Gateway**: recién después, aislar si el origen está en `sync-ai-gateway`.

Resumen corto:

> Primero arreglar la reanudación. Después investigar el corte.

Eso reduce dolor de usuario rápido y evita perseguir infraestructura antes de
resolver el contrato UX más obvio.
