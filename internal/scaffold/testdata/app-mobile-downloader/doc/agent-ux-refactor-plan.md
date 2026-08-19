# Plan de rewrite UX del chat — Agent V2

> Documento vivo. Reemplaza el plan anterior por una estrategia más honesta:
> **rehacer la UI del chat en paralelo** sin reescribir el backend del agente.

## 0. Decisión

La UX actual del chat está demasiado degradada para seguirla parchando:

- `internal/agent/ui/page.templ` concentra demasiada UI + JS inline.
- El cliente actual tiene demasiadas heurísticas y estado duplicado.
- La paridad con `pi` terminal está lejos.
- Corregir incrementalmente la UI actual probablemente cuesta más que reemplazarla.

### Decisión de arquitectura

Se construirá una **UI nueva en paralelo** bajo **V2**, manteniendo el backend
actual del agente (`application/`, `http/events`, `prompt`, `sessions`, etc.)
como motor.

En otras palabras:

> **mismo backend, nueva UI**.

No se crea por ahora un bounded context nuevo `internal/agentv2/` completo.
Solo se duplica la capa **HTTP/UI** necesaria para iterar sin romper la versión
actual.

---

## 1. Objetivo

Llegar a una UX donde el chat en browser sea usable y confiable, con una base
limpia para crecer, sin seguir cargando la deuda del componente actual.

### Meta del rewrite

La V2 debe cubrir bien el flujo principal:

1. abrir chat
2. crear sesión
3. enviar prompt
4. ver streaming
5. ver tools con output útil
6. abortar
7. ver errores claros
8. mantener scroll estable

Todo lo demás es secundario hasta que esto esté sólido.

---

## 2. Alcance

## 2.1 Qué se rehace

Se rehace desde cero la capa visual del chat:

- shell del chat
- header
- sidebar de sesiones
- feed de mensajes
- input/composer
- barra de estado
- render de assistant/user/tool/error
- cliente JS completo

## 2.2 Qué NO se rehace en esta etapa

Se reutiliza el backend existente:

- `internal/agent/application/*`
- `internal/agent/http/events.go`
- `internal/agent/http/prompt.go`
- `internal/agent/http/sessions.go`
- `internal/agent/http/abort.go`
- `internal/agent/application/history.go`
- `internal/agent/application/render.go` (con ajustes puntuales si hace falta)
- `AgentService`

## 2.3 Qué queda explícitamente fuera de la primera versión

No entra de inicio:

- slash commands
- `@` file picker
- drag & drop / edición de cola
- paste de imágenes
- búsqueda en el feed
- virtualización
- shortcuts avanzados
- selector sofisticado de modelo/provider

Si algo de eso entra después, será sobre una base ya sana.

---

## 3. Estructura propuesta

## 3.1 Rutas

Se crea una ruta nueva y explícita:

- `GET /agent-v2`

No se reemplaza `/agent` hasta validar la V2.

## 3.2 Archivos nuevos

### HTTP

- `internal/agent/http/page_v2.go`
- `internal/agent/http/page_v2_test.go`

### UI

- `internal/agent/ui/v2/page.templ`
- `internal/agent/ui/v2/fragments.templ`
- `internal/agent/ui/v2/state.go`
- `internal/agent/ui/v2/render_register.go`
- `internal/agent/ui/v2/htmlexport.go` (solo si hace falta)

### JS estático

- `internal/agent/ui/v2/static/agent-chat/main.js`
- `internal/agent/ui/v2/static/agent-chat/app.js`
- `internal/agent/ui/v2/static/agent-chat/sse.js`
- `internal/agent/ui/v2/static/agent-chat/feed.js`
- `internal/agent/ui/v2/static/agent-chat/composer.js`
- `internal/agent/ui/v2/static/agent-chat/sidebar.js`
- `internal/agent/ui/v2/static/agent-chat/dom.js`
- `internal/agent/ui/v2/static/agent-chat/state.js`

## 3.3 Regla de diseño

La V2 **no duplica lógica de negocio**.

- La V1 y la V2 pueden convivir.
- El backend del agente sigue siendo uno solo.
- Si una mejora requiere tocar `render.go` o `history.go`, se hace como mejora
  compartida, no como fork de backend.

---

## 4. Principios

1. **Server-rendered first.** El server sigue siendo la fuente de verdad.
2. **Una state machine mínima.** El cliente aplica estado; no inventa estado.
3. **Nada de JS inline gigante.** Todo módulo real.
4. **Rollback trivial.** Si falla V2, `/agent` sigue vivo.
5. **Primero confiabilidad, luego features.**
6. **Menos superficie, mejor UX.**

---

## 5. State machine mínima de V2

La V2 parte con un estado deliberadamente chico:

```js
{
  sessionId: "",
  phase: "idle", // idle | running | error
  connected: true,
  loadingHistory: false,
  pendingPrompt: false
}
```

No se arrastran los flags raros de la V1 salvo que demuestren ser necesarios.

Si aparece un caso real que obligue a agregar estado, se agrega después.

---

## 6. Roadmap real

## Milestone A — Reemplazo estructural (base sana)

### Objetivo

Levantar `/agent-v2` con una UI nueva, limpia y funcional usando el backend
actual.

### Entregables

- [ ] Nueva ruta `GET /agent-v2`
- [ ] `page_v2.go` que renderiza la página nueva
- [ ] `page.templ` nuevo bajo `ui/v2/`
- [ ] JS extraído a módulos reales
- [ ] Sidebar de sesiones funcional
- [ ] Feed funcional
- [ ] Input funcional
- [ ] Crear sesión / enviar prompt / abortar usando endpoints existentes
- [ ] SSE funcionando sobre el contrato actual
- [ ] Tests básicos del handler y render inicial

### Criterio de done

La V2 permite:

- abrir el chat
- crear sesión
- mandar prompt
- ver respuesta completa
- abortar sin romper la UI
- cambiar entre sesiones

Sin pensar todavía en paridad avanzada con `pi`.

---

## Milestone B — Paridad visual útil

### Objetivo

Hacer que la V2 se sienta viva y útil, no solo correcta.

### Entregables

- [ ] Thinking streaming visible
- [ ] Tool execution visible
- [ ] Tool output streaming o al menos resultado final útil
- [ ] Render claro de errores
- [ ] Estado del turno confiable (`idle/running/error`)
- [ ] Scroll estable durante streaming
- [ ] Footer simple con costo/tokens si sale barato

### Backend permitido en este milestone

Se permite tocar compartidos si hace falta:

- `internal/agent/application/render.go`
- `internal/agent/application/history.go`
- `internal/agent/ui/...` compartido si conviene

### Criterio de done

Un turno real con tools se entiende visualmente sin mirar logs ni debug modal.

---

## Milestone C — Productividad

### Objetivo

Agregar capacidades tipo `pi` terminal solo después de que el flujo base sea
bueno.

### Candidatos

- [ ] Slash commands
- [ ] `@` file picker
- [ ] Selector de modelo
- [ ] Cola visible
- [ ] Copy de mensajes
- [ ] Mejoras de shortcuts

### Regla

Nada entra aquí si Milestone A o B sigue inestable.

---

## 7. Orden de implementación sugerido

## Paso 1 — levantar la página V2

- copiar el wiring mínimo de `page.go`
- montar `/agent-v2`
- render inicial con layout limpio
- cero streaming complejo todavía

## Paso 2 — mover JS a módulos

Primero mover sin mejorar demasiado:

- bootstrap
- SSE
- input
- feed
- sidebar

El objetivo inicial es **desinlinear** y simplificar.

## Paso 3 — recortar la lógica del cliente

Eliminar heurísticas innecesarias.

El cliente debe depender de:

- eventos SSE
- estado visible del DOM
- estado mínimo local

No de temporizadores mágicos salvo que exista un caso real.

## Paso 4 — enriquecer mensajes

Agregar thinking / tool cards / errores claros.

## Paso 5 — decidir si V2 reemplaza V1

Solo cuando:

- el flujo principal sea mejor que `/agent`
- no haya regresiones graves
- el usuario lo valide explícitamente

---

## 8. Riesgos reales

| Riesgo | Mitigación |
| --- | --- |
| Rehacer demasiado y bloquearse | Mantener V2 limitada al flujo principal al inicio |
| Duplicar lógica de negocio entre V1/V2 | V2 reutiliza `AgentService` y endpoints existentes |
| Reintroducir otra state machine gigante | Limitar estado cliente al mínimo |
| Querer paridad total demasiado pronto | Postergar slash commands / picker / cola / uploads |
| Mantener V1 y V2 mucho tiempo | Definir criterio claro de reemplazo tras Milestone B |

---

## 9. Criterios de éxito

La V2 está lista para reemplazar a la V1 cuando:

- [ ] crear sesión funciona consistentemente
- [ ] prompt + streaming funcionan consistentemente
- [ ] abort funciona
- [ ] errores son entendibles
- [ ] tool output es visible
- [ ] el usuario prefiere `/agent-v2` a `/agent`

---

## 10. Próximo paso concreto

Primera entrega aprobable:

### PR 1

- `GET /agent-v2`
- `internal/agent/http/page_v2.go`
- `internal/agent/ui/v2/page.templ`
- `internal/agent/ui/v2/state.go`
- `internal/agent/ui/v2/static/agent-chat/main.js`
- shell básica
- lista de sesiones
- input
- render de feed inicial
- conexión SSE básica

Eso basta para arrancar.

---

## 11. Regla final

No intentar “arreglar” la V1 mientras nace la V2.

La V1 queda en modo mantenimiento:

- solo fixes críticos
- nada de features nuevas
- toda mejora UX neta va a V2

> La estrategia no es refactor eterno.
> Es **construir un chat nuevo encima del backend existente** y migrar cuando
> esté mejor.
