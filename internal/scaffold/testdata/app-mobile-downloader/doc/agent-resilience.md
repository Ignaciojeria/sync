# Agent Worker — patrones de resilencia

Desde el incidente del 502 del 2026-07-02 (worker crasheó silenciosa
al manejar un SSE lento y el browser sólo veía 502), el worker
incorporó tres capas de defensa. Este doc es la versión humana de
esas decisiones, así cuando alguien modifique `cmd/agent-worker` o
`pkg/agent/worker/handlers/` sabe qué garantías debe mantener.

## Las tres capas (worker side)

### 1. Tier 1 — El worker NO muere en silencio

`cmd/agent-worker/main.go` hace:

- `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` — ante
  SIGTERM/SIGINT, en vez de morir abruptamente, se ejecuta un
  `srv.Shutdown(ctx)` con timeout de 15 s. Esto cierra las conexiones
  SSE ordenadamente, libera sockets y permite que el orquestador
  (`scripts/run-all.sh stop`) y air reinicien limpios.
- `defer recover()` en `main()` — si algo del bootstrap panica, se
  loguea con stack trace completo y `os.Exit(1)` explicito. Antes el
  proceso desaparecía en silencio.
- Middleware `recoverPanic` envuelve **todos** los handlers del mux.
  Si una goroutine de servicio panica, queda logueado, devuelve 500,
  y el proceso sigue vivo. Antes esto mataba al worker entero.
- HTTP server con `ReadHeaderTimeout: 10s`, `IdleTimeout: 120s`,
  `WriteTimeout: 0` (intencional: las conexiones SSE son long-lived).
  Evita goroutines zombies por Slowloris o clientes que cuelgan el
  socket.

**Garantía observable**: cualquier panic en una request queda en el
log con el método, el path, el cliente y el stack completo.

### 2. Tier 2 — El SSE es resumible

`pkg/agent/worker/handlers/events.go` y `journal.go`:

- Cada evento que el worker emite hacia el SSE también queda
  persistido en `$AGENT_EVENTS_DIR/<sid>.jsonl` (default
  `tmp/agent-events`) como `{seq, kind, payload, time}` en una línea
  por evento. El seq es monotónico por sesión — no es un UUID, es
  literalmente `1, 2, 3, ...` en orden de emisión.
- Cada mensaje SSE lleva `id: <seq>` además de `event: pi` y
  `data: <payload>`. El browser guarda el último id que vio.
- Cuando el browser reconecta (por network blip, tab suspendida, o
  porque el worker reinició), manda el header `Last-Event-ID: <n>`
  (con `?resume=<n>` como fallback por si un proxy intermedio filtra
  headers). El handler lee `since = max(header, query)` y reenvía
  primero la cola del journal con seq > since, y después continúa
  con el stream en vivo.
- Race condition entre dos clientes SSE de la misma sesión: el
  mutex proceso-local del journal serializa los appends. El
  seq resultante puede no coincidir con lo que ve un cliente si
  arrancan al mismo tiempo, pero cada cliente recibe la misma
  historia (cualquier seq que un cliente vio, el otro también si
  reconnecta). El mutex basta porque un user normalmente tiene
  una sola tab por sesión.

**Garantía observable**: si el browser se cae 2 s y vuelve, no
pierde ningún evento entremedio.

### 3. Tier 3 — Reconnect con backoff vivo en el browser

`pkg/agent/ui/page.templ`:

- Cuando el SSE termina sin abort externo, `keepStreamAlive` reintenta
  con backoff exponencial: 1 s, 2 s, 4 s, 8 s, 16 s, max 30 s. Jitter
  ±20 % para que dos tabs del mismo user no sincronicen los
  timestamps.
- El contador de intentos se resetea a 0 después de cada
  conexión exitosa (status 200 + stream leído al menos un chunk).
- `streamGeneration` se incrementa cuando el usuario cambia de
  sesión, así un reconnect que queda en cola no se confunde con el
  loop de la sesión anterior.

**Garantía observable**: una caída de 30 s del worker = 5-7
intentos en vez de 30.

## Escenarios de recuperación cubiertos

| Escenario | Comportamiento |
|---|---|
| Worker panics en una request | Log con stack, request devuelve 500, worker sigue |
| Worker se reinicia (air rebuild / deploy) | Suscriptores SSE caen; browser hace backoff + `Last-Event-ID` resume |
| Browser pierde network 5 s | Al volver, reconecta con backoff, recibe cola del journal |
| Browser se suspende 5 min | Mismo: `Last-Event-ID` apunta al último seq; journal tiene todo |
| Worker se cae y vuelve a 1 min | Browser reintenta con backoff; cuando vuelve, resume desde el journal |
| IdP de Casdoor rota claves | `keyfunc.NewDefaultCtx` refresca en background ~15 min; durante esa ventana puede haber 401s por signature inválida |
| Token JWT venció | `/agent/auth` en el web-server dispara refresh contra el IdP, se actualiza el `IDToken` en DB y el browser recibe el nuevo Bearer |

## Lo que sigue (Tier 4 — NO recomendado para este proyecto)

- **Multi-tab sync** (varias tabs del mismo user). ElectricSQL o
  CRDTs sobre Postgres cabrían acá. Pero es overkill mientras
  tengas single-user / single-tab.
- **Migrar jsonl a Postgres** con table por sesión. Vale cuando el
  jsonl crece mucho (cientos de miles de eventos) y querés queries
  sobre el historial. Hoy el formato es append-only con seek lineal.
- **Persistencia distribuida** (si corrés múltiples workers detrás
  de un balanceador). Hoy la persistencia es local al proceso; con
  múltiples workers habría que coordinar por DB.

## Cómo modificar el worker sin romper las garantías

Al tocar `cmd/agent-worker/main.go`:
- Mantené `recoverPanic` envolviendo el mux. Sin él, cualquier panic
  en pirpc o en un handler mata el proceso.
- Mantené `signal.NotifyContext` + `srv.Shutdown` con timeout.
  Esto es lo que hace que `scripts/run-all.sh stop` sea predecible.

Al tocar `pkg/agent/worker/handlers/events.go`:
- Cada `writeSSERaw` debe llevar `id: <seq>` para que el journal /
  `Last-Event-ID` funcione. Si emitís un mensaje sin id, el
  browser no puede trackearlo.
- Cada evento emitido va al journal (`journal.append` antes del
  `writeSSERaw`). Sin esta línea, restart del worker = estado
  perdido.
- El mutex del journal existe por una razón: dos SSE handlers
  simultáneos sobre la misma sesión no deben competir por el seq.

Al tocar `pkg/agent/ui/page.templ`:
- No cambies `keepStreamAlive` para quitar el backoff: es lo que
  evita martillar el worker cuando está caído.
- `lastEventIds` debe seguir siendo por sesión (no global) — el
  user puede cambiar de tab y la nueva sesión no debe arrastrar
  eventos viejos.
