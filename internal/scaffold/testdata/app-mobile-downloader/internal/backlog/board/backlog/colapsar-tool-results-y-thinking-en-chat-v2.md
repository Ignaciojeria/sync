---
type: backlog/card
title: Chat UX: colapsar tool results, file reads grandes, y thinking
description: Hoy el feed del chat mete inline el output completo de
  cada tool, las lecturas de archivos enteros, y el thinking del
  agente. El usuario scrollea 50KB de transcript antes de ver la
  respuesta real. Hace falta colapsar por default + mostrar
  preview + affordance de "click para expandir".
status: backlog
priority: P2
timestamp: 2026-07-19T02:20:00Z
source: user
tags: [agent, ux, chat, frontend]
---

# Chat UX: colapsar tool results, file reads grandes, y thinking

## Contexto

Hoy el feed del chat V2 (`internal/agent/ui/v2/`) renderiza
inline y sin límite el output de cada tool que ejecuta el
agente. Una investigación típica del agente (1 `honcho_search`
+ 2 `bash: ls` + 1 `read: archivo_de_264_líneas` + 1 `read:
otro_archivo_de_200_líneas` + varios `bash: grep`) ocupa
~50KB de transcript visible antes de que el usuario vea la
respuesta final del agente. El problema se ve en cualquier
turno donde el agente decide investigar antes de responder.

El card `doc/agent-ux-refactor-plan.md` ya lista los items
"Tool output streaming o al menos resultado final útil" y
"Tool execution visible" como pendientes, pero no se atacaron.

## Acceptance Criteria

- [ ] Cada `tool_result` se renderiza **colapsado por default**
      con un preview de las primeras 3-5 líneas + un
      `<details>` que el user expande con click.
- [ ] Cuando el tool es `read` (lectura de archivo) y el
      content tiene >50 líneas, el preview muestra **solo el
      path + line count** ("`internal/agent/foo.go` · 264
      líneas · click para ver"). Sin previews de medio archivo.
- [ ] Cuando el tool es `read` y el content es <50 líneas, se
      muestra el contenido completo inline (no rompemos el
      caso de leer archivos cortos).
- [ ] Cuando el tool es `bash` y el output es >10 líneas, se
      muestran las primeras 5 + "..." + indicador de
      "N líneas más".
- [ ] El bloque `thinking` del agente se renderiza **colapsado
      por default** (revierte el cambio reciente que lo dejó
      expandido). Mismo `<details>` que ya existe en
      `fragments.templ`, sólo cambiar `open` por default.
- [ ] El contador de líneas de los outputs colapsados es
      client-side y se calcula al expandir (no recalcula
      render).
- [ ] Los botones de Copy siguen funcionando sobre el contenido
      completo, no sólo el preview.
- [ ] Los tests E2E del chat siguen verdes (no rompemos el
      flow de regenerate, abort, etc.).
- [ ] Acceptance manual: un turno con 5+ tool calls + 1 file
      read grande + 1 thinking block se entiende **sin
      scrollear más de 1 pantalla** hasta la respuesta final.

# Plan

## Fase A — Server-side truncation (templ)

1. En `internal/agent/ui/v2/fragments.templ`, helper
   `previewToolOutput(toolName, content, maxLines)` que
   decide el preview según tool name y tamaño:

   | tool | content | preview | expand-on-click |
   |---|---|---|---|
   | `read` | <50 líneas | full | n/a |
   | `read` | ≥50 líneas | `path · N líneas` | full content |
   | `bash` | ≤10 líneas | full | n/a |
   | `bash` | >10 líneas | primeras 5 + "… (N más)" | full output |
   | otros | ≤10 líneas | full | n/a |
   | otros | >10 líneas | primeras 5 + "… (N más)" | full output |

   El `path` para el caso `read` viene de los args del tool
   (key=path) o del primer `<line>1: <code>` que tenga el
   output. Si no se puede inferir, fallback a "file read · N
   lines".

2. Refactor del case `tool_result` para usar
   `previewToolOutput` + `<details>`. El render es:

   ```html
   <div class="v2-tool-result" data-truncated="true|false">
     <div class="v2-tool-result-label">output · {toolName}</div>
     <details class="v2-tool-result-details">
       <summary>{preview}</summary>
       <pre class="v2-tool-output"><code>{fullContent}</code></pre>
     </details>
   </div>
   ```

   Sin `<details>` cuando `data-truncated=false` (caso
   pequeño). El user no ve affordance si no hay nada que
   expandir.

3. Cambiar el case `thinking` de `<details class="v2-thinking"
   open>` a `<details class="v2-thinking">` (sin `open`).
   Reverte a colapsado por default. El `<summary>` ya existe
   y dice "Pensando · N caracteres" o similar.

4. CSS en `standalone.templ`:
   - `.v2-tool-result-details > summary` con el mismo estilo
     que `.v2-thinking > summary` (muted, hover bg, cursor
     pointer).
   - El `<pre>` adentro del `<details>` mantiene el
     `max-height: 24rem; overflow-y: auto` actual para
     outputs grandes.
   - Padding/margin del `<details>` para que no rompa el
     spacing actual del feed.

## Fase B — Copy button sobre content completo

El botón Copy (`data-copy-button` en cada item) hoy copia
`item.Text` que es el texto plano del item. Para los
`tool_result`, queremos que copie el **output completo**,
no sólo el preview. Hay dos opciones:

- Opción 1: el atributo `data-copy-text` del botón trae el
  full content server-side, y el handler JS lee ese atributo
  si está presente.
- Opción 2: el server pone el full content en un `<template>`
  oculto y el handler lo lee.

Opción 1 es más simple. Un hidden `data-full-output` en el
`<pre>` y el handler del Copy lo lee. El preview no cambia.

## Fase C — Tests

1. Unit test del helper `previewToolOutput` (12 casos: 3
   tools × 4 tamaños).
2. E2E test: crear un script `scripts/test-chat-ux.sh` (o
   agregar a `test-honcho-conversation.sh`) que abra una
   sesión, mande un prompt que fuerce file reads grandes, y
   verifique con `curl` + `grep` que el HTML tiene
   `data-truncated="true"` y `<details>` colapsado.
3. Acceptance manual: ver el feed con un turno real y
   confirmar que la respuesta final está a 1 pantalla de
   distancia del user prompt.

# Out of Scope

- Streaming del tool output (sólo mostrar el output final).
  Hoy el output es único al final del tool; stream sería
  trabajo de render.go, no de UX.
- Highlight de syntax diferenciado por tool (bash=shell,
  read=auto-detect). Eso ya lo hace highlight.js con
  auto-detect; no hace falta discriminar.
- Cambiar el orden de los items del feed (tool antes del
  tool_result, etc.). Eso es funcional, no UX.
- Indicador de "el agente está trabajando" arriba del
  último turn. Es otra card de UX; este card es sólo sobre
  el contenido de los items.

# Riesgos

- **Regresión en copy**: si el botón Copy no se actualiza
  para tomar el full content, los users van a copiar sólo
  el preview por accidente. Mitigado por tests E2E.
- **Regresión en search**: el find-in-page del browser
  matchea contra el DOM, no contra el contenido colapsado.
  Si el user busca un string que está en un tool_result
  colapsado, el browser lo encuentra igual (es parte del
  DOM) pero el user tiene que expandir manualmente para
  verlo. Decisión consciente: searchabilidad gana sobre
  prolijidad visual. Documentar.
- **Cambio de behavior en thinking**: revertir el `open` por
  default puede frustrar a users que se acostumbraron al
  thinking visible. Mitigado: el `<summary>` siempre muestra
  meta del thinking (chars, primera línea) para que se
  pueda peever sin expandir.

# Links

- Plan general de UX: `doc/agent-ux-refactor-plan.md` §M-B.2
  y §M-C.
- Render actual de tool_result: `internal/agent/ui/v2/fragments.templ`
  case `"tool_result"` y case `"thinking"`.
- CSS actual: `internal/agent/ui/v2/standalone.templ` líneas
  945-1000 (tool output) y 1007-1060 (thinking).
- Apply fragment en cliente: `internal/agent/ui/v2/static/agent-chat/feed.js`
  función `applyFragment`.
