---
description: 'Cutover UI-only: mover la shell v2 a /agent (sacando /agent-v2), borrar la UI v1 (internal/agent/ui/{page,fragments,standalone,providers}.templ, state.go, htmlexport.go, render_register.go) y dejar de exportar el flag AGENT_V2_ENABLED. Renombrar RegisterV2→Register y eliminar RegisterAllLegacy, getMessagesHTML y providersPage. Los handlers de datos (/agent/sessions/*) se mantienen porque v2 los sirve directo. La remoción total de los handlers queda fuera de scope (otra card si se quiere partir el agente a otro proceso).'
priority: P2
source: user
status: done
timestamp: "2026-07-18T23:10:38Z"
title: Deprecar agent v1 y dejar solo v2
type: backlog/card
---
# Deprecar agent v1 y dejar solo v2

# Decisión de scope (acordada)

**Cutover UI-only.** Mover la shell v2 a `/agent`, borrar la UI v1
(`internal/agent/ui/{page,fragments,standalone,providers}.templ` +
`state.go` + `htmlexport.go` + `render_register.go`), y dejar de
exportar el flag `AGENT_V2_ENABLED`. Los **handlers de datos**
(`/agent/sessions/{id}/*` para SSE, prompt, abort, merge, preview,
CRUD) se mantienen: v2 los consume via forwarders in-process y el
registry per-session de renderers. "v1" deja de ser una shell visible
y pasa a ser backend sin cara.

Fuera de scope: mover los handlers de datos a otro prefijo o proceso
(queda para otra card si en el futuro se quiere partir el agente).

# Acoplamientos v1↔v2 que hay que romper (hallazgos del audit)

1. **`agentui.RegisterFragmentRenderer()` en `cmd/api/main.go:190`**
   registra el renderer **fallback** (v1) en el registry de
   `agentapp`. Si nadie llama a esto y nadie abre una página v2, el
   SSE cae al comportamiento viejo (envelope crudo). Tras el cutover
   el renderer fallback deja de existir; o (a) se elimina la llamada
   y se exige que toda sesión tenga su renderer v2 registrado antes
   del primer fragment, o (b) se hace que el fallback también renderice
   v2. Recomendado: **(a)**, porque la rama fallback es un fallback de
   seguridad que no debería usarse en el flujo normal.

2. **`getMessagesHTML` en `internal/agent/http/sessions.go:122-152`**
   renderiza el feed inicial de la conversación con
   `agentui.RenderItem(item).Render(...)` — siempre HTML v1,
   ignorando el registry per-session. v2 NO consume este endpoint
   (carga la historia directo en el page render), así que tras el
   cutover queda muerto. **Borrarlo junto con la UI v1.**

3. **`pageHandler` en `internal/agent/http/page.go:29`** monta
   `GET /agent`, `GET /agent/home`, `GET /agent/providers`,
   `GET /agent/login`. `providersPage` (v1) no tiene equivalente v2
   — `/agent/providers` y `/agent/login` desaparecen con v1. La home
   `/agent/home` también es v1-only (v2 la cubre con
   `/agent-v2/home`, que pasa a `/agent/home` con el renombre).

4. **`page_v2_e2e_v1_test.go:30`** usa
   `registerAllLegacyWithEditor(srv, manager, nil, OIDCRefreshConfig{}, noopEditor)`
   para validar que el endpoint v1 backend funciona. Si lo borramos
   en fase 1, el test no compila. Opciones: (a) renombrarlo a
   `sessions_backend_e2e_test.go` y reemplazar
   `registerAllLegacyWithEditor` por el wiring nuevo de `register`
   (el que monta `/agent` v2 + data handlers), (b) mover el archivo
   a `done/`. Recomendado: **(a)**, sigue siendo regression baseline
   útil.

# Plan de implementación

## Paso 1 — Renombrar wiring y mover handlers v2 a `/agent`

**Archivos a tocar:**
- `internal/agent/http/page_v2.go` → renombrar a `page.go` (sobreescribir
  el actual). Sacar el sufijo `-v2` de todos los paths
  (`/agent-v2/*` → `/agent/*`, `/agent-v2/sessions/{id}/events` →
  `/agent/sessions/{id}/events`, etc.).
- `internal/agent/http/register.go`:
  - Borrar `Register()` (el viejo, el que solo montaba v1). Esto es
    API pública del módulo: si algún embedder externo la importa,
    borrarla es breaking change. Si el repo solo la usa `cmd/api`,
    es seguro.
  - Renombrar `RegisterV2` → `Register`. Quitar el parámetro `enabled
    bool`; el caller siempre lo invoca.
  - `RegisterAllLegacy` ya no existe: su contenido se divide entre
    `Register` (UI + data) y los handlers de auth/merge/preview que
    ya están separados.
- `cmd/api/main.go:registerAgent`:
  - Borrar la línea `agentui.RegisterFragmentRenderer()` (190) — v2
    se auto-registra via `RegisterRendererForSession` en cada entry point.
  - Reemplazar el par `RegisterAllLegacy` + `RegisterV2` por un único
    `agenthttp.Register(s, manager, sessionLookup, oidcCfg, requireEditor, sessionCostSvc)`.
  - Borrar `agentV2Enabled()` y la lectura de `AGENT_V2_ENABLED`.
  - Sacar el log "V2 chat enabled/disabled".
  - Sacar el log "legacy mode enabled (/agent + data endpoints in cmd/api)"
    (ya no aplica: todo es la nueva `Register`).
  - Borrar la llamada `agenthttp.SetV2ForwardMux(s.Mux)` (línea 160)
    y el bloque de comentario que la precede. Ya no aplica porque
    no hay forwarders v2→v1; v2 sirve los datos directo.
  - Mover la limpieza `agentuiv2.ClearRendererForSession(sessionID)`
    que hoy vive en `page_v2.go:80-87` (DELETE `/agent-v2/sessions/{id}`)
    al handler de data del nuevo `Register`. Sin esto, los DELETEs
    vía `/agent/...` (que es la única ruta post-cutover) dejan
    renderers huérfanos en `globalRegistry.perScope` (memory leak
    chico pero acumulativo).
  - `.env` (línea 35): borrar `AGENT_V2_ENABLED=true` (ya no se lee).

**Comprobación intermedia:**
`go build ./...` compila. `GET /agent` renderiza la shell v2.
`POST /agent/sessions`, `POST /agent/sessions/{id}/prompt`,
`GET /agent/sessions/{id}/events`, `POST /agent/sessions/{id}/abort`,
`POST /agent/sessions/{id}/merge`, `GET /agent/sessions/{id}/preview/...`
siguen funcionando (ya no hay forwarder, se sirven directo desde
los handlers de data).

## Paso 2 — Borrar la UI v1

**Archivos a `git rm`:**
- `internal/agent/ui/page.templ` + `page_templ.go` (generado)
- `internal/agent/ui/fragments.templ` + `fragments_templ.go`
- `internal/agent/ui/standalone.templ` + `standalone_templ.go`
- `internal/agent/ui/providers.templ` + `providers_templ.go`
- `internal/agent/ui/state.go` + `state_test.go`
- `internal/agent/ui/htmlexport.go`
- `internal/agent/ui/render_register.go`
- `internal/agent/ui/page_templ_test.go`

**Archivos a editar (eliminar referencias a `agentui`):**
- `internal/agent/http/sessions.go:5` (import `agentui`),
  `sessions.go:146` (uso de `agentui.RenderItem`).
- `internal/agent/application/render.go:1-99` (comentarios sobre
  "V1" / "fallback legacy"). Reescribir el doc-comment de
  `SetFragmentRenderer` y `registry` para no hablar de V1/V2: hoy el
  registry es per-session con un fallback opcional.
- Cualquier otro `rg "gitinittest5/internal/agent/ui"` (sin `/v2`)
  en `*.go` que no sea de los archivos borrados.

**Comprobación intermedia:**
`rg "internal/agent/ui\"" .` (sin v2) no devuelve hits.
`go build ./...` compila. `templ generate` no regenera nada
nuevo.

## Paso 3 — Limpiar tests

- `internal/agent/http/page_v2_e2e_v1_test.go` → renombrar a
  `sessions_backend_e2e_test.go`. Reemplazar la llamada a
  `registerAllLegacyWithEditor(srv, manager, nil, ...)` por
  `register(srv, manager, nil, ...)` (el nuevo `register` que
  montamos en paso 1). Actualizar el comentario de cabecera para
  reflejar que es regression del backend de datos (no de la UI v1).
- `internal/agent/http/page_v2_e2e_test.go` y `page_v2_test.go`:
  cambiar `ts.URL+"/agent-v2/..."` por `ts.URL+"/agent/..."` y los
  paths de forwarders `/agent-v2/sessions/...` por
  `/agent/sessions/...`. Renombrar archivos a `agent_e2e_test.go` y
  `agent_test.go` para limpiar el sufijo `-v2`.
- `internal/agent/http/page_v2_e2e_isolated_test.go` → renombrar a
  `agent_e2e_isolated_test.go`. Migrar cualquier path `/agent-v2/...`
  a `/agent/...`.
- `internal/agent/http/events_sse_test.go:46` llama a
  `agentui.RegisterFragmentRenderer()`. Como esa función deja de
  existir en el paso 2, **borrar esa línea**. El test sigue siendo
  válido: verifica el flujo SSE del backend, solo que ahora el
  renderer se inyecta via `agentuiv2.RegisterRendererForSession` en
  el path del page handler. Si el test solo cubre el caso "sin
  page handler previo" (renderer per-session nunca seteado), evaluar
  si conviene agregar un setUp que llame a `agentuiv2.RegisterRendererForSession`
  o usar un testFragmentRenderer propio.
- `internal/agent/http/sessions_messages_test.go` → **borrar**. El
  endpoint `/agent/sessions/{id}/messages` desaparece en el paso 1
  junto con `getMessagesHTML`. Si algún assert cubre lógica que sí
  sobrevive, migrarlo al test del feed inicial (`agent_e2e_test.go`).
- `internal/agent/application/render_streaming_test.go:82` menciona
  `cmd/api/main.go:RegisterFragmentRenderer` en un comentario.
  Actualizar el comentario: el renderer ahora se inyecta via
  `agentuiv2.RegisterRendererForSession` per-session.

**Comprobación intermedia:**
`go test ./...` verde.

## Paso 4 — Actualizar docs

- `doc/agent-runtime.md`: borrar secciones sobre la fase de
  construcción, el flag `AGENT_V2_ENABLED`, la coexistencia v1/v2.
  Reescribir la sección de UI del agente en pasado: "el chat del
  agente vive en `/agent`, renderizado por `internal/agent/ui/v2/`
  (cutover 2026-07-XX, reemplazó a la UI v1)".
- `AGENTS.md`: revisar la sección "Módulo agente" — no menciona
  v1, pero el ejemplo de `Register` puede haber quedado con el
  wiring viejo. Actualizar al nuevo.
- `STRUCTURE.md`: regenerar con `bash scripts/generate-structure.sh`
  (existe en el repo). Debería mostrar que `internal/agent/ui/`
  solo tiene `v2/`.
- Comentarios de código:
  - `internal/agent/http/page.go` (nuevo, el ex-`page_v2.go`):
    borrar "vive en /agent-v2 mientras v1 sigue en /agent".
  - `internal/agent/ui/v2/state.go` y `embed.go`: borrar referencias
    a "V1" en comentarios.

**Comprobación intermedia:**
`rg -nE "agent-v2|AGENT_V2_ENABLED|registerV2" .` no devuelve hits
fuera de git history.

## Paso 5 — Verificación final

- [ ] `go build ./...` sin warnings.
- [ ] `go test ./...` verde, sin `t.Skip`.
- [ ] Smoke manual con `air`:
  - [ ] `GET /agent` → shell v2.
  - [ ] `POST /agent/sessions` (crear) → 200, session ID en body.
  - [ ] `POST /agent/sessions/{id}/prompt` → 200.
  - [ ] `GET /agent/sessions/{id}/events` → SSE con `kind:fragment`
        y HTML v2 (clase `v2-bubble` o el marker que use el template,
        NO la clase v1).
  - [ ] `POST /agent/sessions/{id}/abort` → 200.
  - [ ] Click "Open preview" en el Worktree Inspector → abre el
        preview del worktree.
  - [ ] Click "Regenerate" en una respuesta previa → borra el feed
        desde el último user prompt y vuelve a streamear.
- [ ] `rg -nE "\bv1\b|\bV1\b" internal/agent/ cmd/api/` solo matchea
      comentarios históricos del commit.
- [ ] `rg -nE "agent-v2|AGENT_V2_ENABLED" .` no devuelve hits.

# Criterios de aceptación

- [ ] `internal/agent/ui/` solo contiene el subdirectorio `v2/`.
- [ ] `GET /agent` renderiza shell v2; `/agent-v2` no existe.
- [ ] No existe el flag `AGENT_V2_ENABLED` en el código.
- [ ] No existe la función `RegisterV2` ni `RegisterAllLegacy` (solo
      `Register`).
- [ ] No existe `getMessagesHTML` ni el endpoint `/agent/sessions/{id}/messages`.
- [ ] `cmd/api/main.go:registerAgent` no llama a
      `agentui.RegisterFragmentRenderer`.
- [ ] `go test ./...` verde, sin tests skipped.
- [ ] Smoke E2E manual completo funciona.

# Riesgos

- **Renderer per-session obligatorio:** si tras el cutover alguien
  consume el SSE sin haber pasado por un page handler v2 (ej: un
  cliente custom o un test que no llama a `RegisterRendererForSession`),
  el registry devuelve el fallback (que ya no existe) y el SSE
  emite envelope crudo. Mitigación: si en el paso 1 decidimos
  dejar el fallback apuntando a v2 (opción b del hallazgo 1), el
  riesgo desaparece a costa de un poco de duplicación. Decisión a
  tomar antes de empezar el paso 1.
- **Worktree Inspector Panel:** los endpoints
  `worktreeInspectorHandler` están atados a `RegisterV2`. Si en el
  paso 1 renombramos a `Register`, verificar que el wiring se
  mantenga y que los tests del inspector (si los hay) sigan
  pasando.
- **Sesiones con renderer stale:** `ClearRendererForSession` solo
  se llama en el handler DELETE. Si una sesión queda huérfana (no
  se borra pero el cliente se fue), el renderer per-session queda
  en memoria hasta que se reinicie el proceso. Bajo (pocas sesiones,
  mapa chico) pero mencionable.
