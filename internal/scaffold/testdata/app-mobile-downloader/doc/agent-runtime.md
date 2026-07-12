# Runtime del agente pi en `testboi1`

> Documento vivo para entender qué partes del módulo `internal/agent`
> sobreviven a un fork del repo sin intervención y qué partes requieren
> ajustes al migrar a otro proyecto (boilerplate). También cubre los
> puntos pendientes para llegar a 12-factor compliance estricto.

> **Nota 2026-07-08:** el modo activo del proyecto volvió a **app única**
> en `cmd/api`. Las secciones sobre BFF / `cmd/agent-worker` /
> `scripts/run-all.sh` quedan como **contexto histórico** de una etapa
> anterior y no describen el flujo actual de dev.

## 0. Pool de runtimes (cambio del 2026-07-03 — baja RAM drástica)

**Antes**: cada chat usado mantenía 1 proceso `pi` vivo
(`map[sessionID]Runtime` en `pkg/agent/application/manager.go`).
Con N chats: N procesos.

**Ahora**: pool de runtimes con cap configurable. Default
`poolSize=1` vía `Manager.WithPoolSize(n)`. Cuando llega un
prompt para una sesión que no tiene slot, se le hace lugar
evictando al LRU (mata el `pi` viejo y respawnea con
`--session=<nuevo>`). Ver `maybeEvictForNewSlot` y `pickLRU` en
el manager.

**Resultado**: `poolSize × RSS de pi` es el techo de RAM para
el agente, no `sesiones × RSS de pi`.

- Con 1 chat: 1 proceso (igual que antes).
- Con 2+ chats: la sesión menos usada recientemente se mata y
  su próxima iteración respawnea (~1–2 s).

**Tests**: `TestManagerPool_DefaultSizeIsOne`,
`TestManagerPool_SizeAllowsConcurrency`,
`TestManagerPool_ReusesSameSessionSlot`,
`TestManagerPool_LRUEvictsOldest`.

Si querés switch en caliente sin matar el proceso, está
disponible el RPC `switch_session` en pi (verificado en
`node_modules/@mariozechner/pi-coding-agent/dist/modes/rpc/rpc-types.d.ts`).
El approach actual (respawn) es más simple y evita el
`cancelled: true` que devuelve pi cuando está mid-turn; upgrade
path documentado en `pkg/agent/application/manager.go` §
`maybeEvictForNewSlot`.

## 1. Lo verificado el 2026-07-02

Comprobado empíricamente con `pi --mode rpc --provider anthropic --model
claude-sonnet` desde una VM con `air` activo:

- El agente pi tiene `read`/`bash`/`write`/`edit` tools y **puede editar
  archivos del repo sin pedir confirmación**: en la prueba le pedimos
  que agregara una línea al top de
  `pkg/agent/infrastructure/pirpc/runner.go` y lo hizo en ~3 s.
- Editar un archivo bajo `pkg/agent/` **disparó el watcher de air
  → rebuild → reinicio del servidor Go**, dejando el proceso `pi`
  huérfano. Esto es lo que el usuario sintió como "se cuelga y deja de
  funcionar".
- El fix aplicado (`pkg/agent/infrastructure/pirpc/sandbox.go`)
  redirige el CWD del proceso pi a `tmp/agent-work/<sessionID>/`,
  excluido del watcher porque `tmp/` ya está en `exclude_dir` de
  `.air.toml`. Misma prueba después del fix: las ediciones quedan en el
  sandbox, el servidor no se reinicia.

## 2. Lo que sobrevive a un fork sin tocar nada

| Capacidad                                              | Estado                                      |
| ------------------------------------------------------ | ------------------------------------------- |
| Sandbox CWD del agente                                 | Funciona: el sandbox es relativo al cwd del binary. |
| SSE keepalive corto (10 s) y reconexión del cliente    | Funciona: no depende de paths.             |
| Persistencia de sesiones en disco                      | Funciona con `AGENT_SESSION_DIR`. Si la env está vacía, cae a `tmp/agent-sessions/` (relativo al cwd del server). |
| Disk store → fallback a memoria silencioso              | Funciona: el server loggea si el disco falla. |
| Anti-hang en `piRuntime.send`                          | Funciona: timeout + kill-and-respawn.       |
| Buffer del subscriber (256) + drop con log             | Funciona.                                  |
| Errores visibles al usuario final (burbuja roja, indicador live/desconectado) | Funciona: son strings JS inertes. |
| Route + middleware (`/agent/*`, requiere editor auth)  | Funciona: está ligado al JWT middleware del proyecto. |
| Pre-warm del runtime en `page.go`                      | Funciona: hace un `Ensure` con ctx 10 s.   |

## 3. Lo que SÍ requiere intervención al forkear

### 3.1. Renombre de módulo Go (obligatorio)

Todos los archivos `.go` tienen el module path
`testboi1/<paquete>`. Al forkear a un proyecto nuevo hay que:

```sh
# desde la raíz del fork
go mod edit -module github.com/mi-org/mi-proyecto
rg -l "testboi1" | xargs sed -i 's|testboi1|mi-proyecto|g'
go mod tidy
```

Esto es convencional en Go pero conviene documentarlo. El boilerplate
debería tener un script `scripts/rename-module.sh` que automatice esto.

### 3.2. CWD relativo al binario, no al proyecto

La constante `SandboxRoot = "tmp/agent-work"` y la default
`tmp/agent-sessions` del disk store se construyen con
`filepath.Abs(...)` o `os.MkdirAll(...)` desde el CWD del proceso
servidor.

- En dev con `air`: el cwd coincide con la raíz del repo. OK.
- En deploy con systemd/k8s/Docker: el cwd del binario puede ser
  `/opt/<app>`, `/var/lib/<app>`, `/app`, etc. **El sandbox va a parar
  ahí, no necesariamente dentro del proyecto**.

Esto ya es una violación del 12-factor #3 (config) y un riesgo de
12-factor #10 (dev/prod parity). Hay que volverlo configurable por env
var y, mejor todavía, anclarlo a un directorio del proyecto que el
boilerplate determine (ver §5).

### 3.3. Excluir `tmp/` en el watcher del nuevo proyecto

El fix anti-rebuild depende de que **el hot-reload del nuevo proyecto
ignore `tmp/`**. Hoy `.air.toml` ya lo hace:

```toml
exclude_dir = ["assets", "tmp", "vendor", "node_modules", ".git", ".einar"]
```

Si el nuevo proyecto no usa `air` (usa `fresh`, `CompileDaemon`,
`make watch`, `bun --watch`, Vite, Next.js, etc.) hay que **editar el
config de su hot-reload** para excluir `tmp/`. Para
boilerplates que no asumen `air`, hay que documentar el contrato en el
README.

### 3.4. Dependencia externa: el binario `pi`

El runner hace `exec.LookPath("pi")` si no se le pasa binary
explícito (`agentpirpc.NewRunner("")`). Para que el boilerplate
funcione sin intervención:

- `pi` debe estar instalado en la VM/imagen del proyecto.
- `pi` debe tener un provider válido en su config (en esta VM fue
  `fireworks/minimax-m3` — dependió del HOME/.pi/agent/extensions).
- Si se quiere un provider determinístico, hay que pasar `Model:
  "anthropic/claude-sonnet-5"` desde `CreateSessionInput` o vía env
  var. Hoy está cableado en `spec.Model` de la sesión. **Se debería
  poder override por env (`AGENT_DEFAULT_MODEL`)**.

### 3.5. Configuración 12-factor que falta

Variables que SÍ existen:

- `AGENT_SESSION_DIR`: ruta del disk store.

Variables que NO existen (deberían):

- `AGENT_SANDBOX_DIR`: raíz del sandbox por sesión.
- `AGENT_PI_BINARY`: ruta al binario pi (default `pi`).
- `AGENT_DEFAULT_MODEL`: provider/model default para sesiones nuevas.
- `AGENT_DEFAULT_PROVIDER`: alternativa explícita al `--provider` de pi.
- `AGENT_REQUIRE_AUTH`: por defecto el agente requiere editor auth;
  para boilerplates que aún no tengan JWT servido, debería poder
  saltarse con un flag explícito, no con el actual
  `AUTH_DISABLED=true` del proyecto (que deshabilita toda la auth
  del server, no sólo el agente).

## 4. Lo que NO está resuelto

1. **El sandbox es un default, no aislamiento real.** Pi puede leer
   el repo y, si el LLM decide usar paths absolutos, puede aún editar
   el repo directo. Para aislamiento fuerte hace falta:

   - **Worktree por sesión**: clonar cada `pi` en
     `tmp/agent-work/<session>/repo/`, los edits van a una rama
     paralela.
   - **Pre-tool-validate hook**: middleware a nivel de tool que valide
     cada `edit`/`write` contra un allowlist antes de ejecutarlo.

   Ambos son trabajo futuro, no implementados.

2. **Hooks de aprobación de pi.** Hoy pi corre con cualquier tool sin
   prompt al usuario. Para un boilerplate "production ready" habría
   que cablear `--approve <ruta>` o un wrapper que pase por un
   approval flow.

3. **Limpieza de procesos pi huérfanos.** El Manager.Close() cierra
   bien, pero si el server recibe SIGKILL (air restart en
   modo `kill`), pi queda vivo hasta el final natural del turn. Un
   proceso supervisor externo (systemd con `KillMode=process`,
   `Type=notify`) o un reapaso con `prctl(PR_SET_PDEATHSIG)` son
   opciones a documentar.

4. **Concurrencia y escalabilidad.** El Manager actual no soporta
   múltiples instancias del server compartiendo estado (las runtimes
   son in-memory). Para 12-factor #8 (concurrency) esto requiere
   mover el session store a Postgres y registrar las runtimes en un
   registry compartido (Redis o similar).

## 5. Checklist para forkear el boilerplate

```sh
# 1. Renombrar el módulo y los imports
go mod edit -module <nuevo-path>
rg -l "testboi1" | xargs sed -i 's|testboi1|<nuevo-path>|g'
go mod tidy

# 2. Asegurar que el hot-reload ignore tmp/
# Si usás air:
rg -q '"tmp"' .air.toml || echo "Agregá 'tmp' a exclude_dir en .air.toml"
# Si usás otra cosa: editar su config equivalente.

# 3. Instalar pi y verificar provider
pi --version
pi -p "Hola"  # confirma que responde

# 4. (opcional) Variables 12-factor
export AGENT_SESSION_DIR=/var/lib/<proyecto>/agent-sessions
# export AGENT_SANDBOX_DIR=/var/lib/<proyecto>/agent-sandbox  # futuro
export AGENT_DEFAULT_MODEL=anthropic/claude-sonnet
# export AGENT_PI_BINARY=/usr/local/bin/pi  # futuro

# 5. Probar que el ciclo no se rompe
curl http://localhost:8000/agent         # debe responder 401 o 200
# (autenticado) Enviar un prompt de prueba que incluya pidele al
# agente que modifique un .go cualquiera del proyecto. Verificar:
#   a. que la edición termina en tmp/agent-work/<session>/, NO en el repo.
#   b. que el server NO se reinicia durante la edición.
#   c. que el burbujeo SSE sigue funcionando después.
```

## 6. Trabajo pendiente para 12-factor estricto

| Factor        | Pendiente para el agente                                                                          |
| ------------- | ------------------------------------------------------------------------------------------------- |
| I. Codebase   | (OK: un repo).                                                                                    |
| II. Deps      | Falta: `pi` no está declarado como dep en `go.mod`. Es un bin externo y eso es OK por 12-factor, pero conviene un script `scripts/install-pi.sh`. |
| III. Config   | **Falta**: `AGENT_SANDBOX_DIR`, `AGENT_PI_BINARY`, `AGENT_DEFAULT_MODEL`, `AGENT_DEFAULT_PROVIDER`, `AGENT_REQUIRE_AUTH`. |
| IV. Backing services | `pi` se trata como recurso adjunto (OK). El session store es configurable por env (OK). |
| V. Build/Release/Run | El script de build vive sólo en `.air.toml`. Falta script de release reproducible. |
| VI. Processes | El manager no es stateless entre réplicas (runtimes viven en memoria).                              |
| VII. Port     | OK (fuego + `PORT` env).                                                                          |
| VIII. Concurrency | Runtimes en memoria; no escala.                                                            |
| IX. Disposability | Falta asegurar que `pi` muera con el server (PR_SET_PDEATHSIG o supervisor externo). |
| X. Dev/prod parity | `pi` resuelve provider según env/CLI; si difiere entre dev y prod se rompe idéntico. |
| XI. Logs      | OK (`slog` a stdout).                                                                             |
| XII. Admin    | OK (los hooks de shutdown + health endpoints existentes).                                         |

## 7. Cómo probar que el boilerplate funciona sin intervención

1. Forke a un proyecto nuevo sin tocar nada del agente.
2. Aplicar §5 pasos 1–4.
3. Levantar con `air`.
4. Mandar al agente el prompt:
   *"Edita `internal/<cualquier>/foo.go` y agregale el comentario
   `TEST_BOILERPLATE` al inicio."*
5. Verificar que:
   - La edición aparece en `tmp/agent-work/<session>/foo.go`, no en
     `internal/<cualquier>/foo.go`.
   - El server no se reinició (sin entradas de
     `rebuilding/trigger` en `.air.log`).
   - El SSE sigue transmitiendo (el indicador del header sigue en
     "live").
   - Las runtimes previas murieron limpio (no quedan `pi` zombies en
     `ps aux | grep pi`).
6. Si todo eso pasa, el boilerplate es bueno para 12-factor.

## 8. Referencias

- Código del fix de sandbox:
  `pkg/agent/infrastructure/pirpc/sandbox.go`
- Proceso afectado:
  `pkg/agent/infrastructure/pirpc/process.go`
- Página que arranca el agente con CWD vacío:
  `pkg/agent/http/page.go`
- Configuración de air que debe sobrevivir al fork:
  `.air.toml` (línea `exclude_dir`)
- Plan original del módulo:
  `AGENT_IMPLEMENTATION_PLAN.md`

## 9. Layering módulo y opt-out (a partir del 2026-07-02)

El agente vive en `pkg/agent/` (no más `internal/agent/`) para que un
proyecto derivado pueda tratarlo como capability removible sin tocar
el resto del wiring. La estructura:

```
pkg/agent/
├── application/         ← contrato público: AgentService interface + Manager
├── http/                ← handlers HTTP (Register acepta AgentService)
├── infrastructure/
│   ├── pirpc/           ← spawn pi, sandbox, validated apply (futuro)
│   ├── disk/            ← session store persistente
│   └── memory/          ← session store en memoria
└── ui/                  ← templates templ del chat
```

### 9.1. Contrato público

`Application.AgentService` es la interfaz con la que el host habla.
Declarada en `pkg/agent/application/manager.go`:

```go
type AgentService interface {
    List/Get/Create(ctx, …) (…)
    Ensure(ctx, id) error
    Prompt/Steer/Abort(ctx, id, msg) error
    Subscribe(ctx, id) (<-chan Event, func(), error)
    Close() error
}
```

El `*application.Manager` la satisface (asserted via
`var _ AgentService = (*Manager)(nil)` en tiempo de compilación). El
host (cmd/api/main.go) tipa la variable como `agentapp.AgentService`,
no como tipo concreto.

### 9.2. Opt-out vía env var

Para forkear el boilerplate sin el agente:

```sh
AGENT_ENABLED=false ./tmp/main
```

Cuando la var está en `false|0|no|off` (case-insensitive), el host
no registra endpoints `/agent/*`, no spawnea `pi`, y el resto del
servidor funciona idéntico. Ver `TestNewAgentDepsRespectsAGENTEnabled`
para la tabla de valores aceptados.

No hay que eliminar código: el flag deja el wiring congelado para
activarse de nuevo cuando se necesite.

### 9.3. Lo que falta para full separation

Hoy `pkg/agent/http` y `pkg/agent/application` todavía importan tipos
del host (`internal/shared/server`, `internal/ui/layout`,
`internal/auth/middleware`). El path de imports llega via go's `internal`
rule porque `pkg/agent` está dentro del mismo módulo.

Para llegar al "nivel 2" del análisis original (módulo Go separado
con `go.work`, dependencias invertidas vía interfaces en lugar de
imports directos), falta:
1. Mover `internal/shared/server`, `internal/ui/layout`,
   `internal/auth/middleware` a paths non-`internal`.
2. Definir en `pkg/agent` interfaces `HostRouter`, `HostMiddlewareFunc`,
   `LayoutResolver` (análogas a `AgentService`).
3. `pkg/agent/http` consume las interfaces, no los tipos concretos del
   host.
4. `cmd/api/main.go` provee adapters (fuego → HostRouter, etc.).

Esto queda como TODO; no hace falta para que el boilerplate funcione
y se puede hacer incrementalmente sin romper llamadas existentes.

## 10. Topología de tres procesos (a partir del 2026-07-02, step 1)

El "se reinicia y contamina al agente" deja de existir cuando separamos
el host web de la runtime del agente. La arquitectura mínima que lo
logra tiene tres binarios:

```
   browser
      │
      ▼
┌──────────┐   gateway estable, proxy inverso 'tonto'
│   BFF    │   cmd/bff        listen :8000
│ :8000    │   Hand-coded, FROZEN, NO tocado por air.
└──┬────┬──┘
   │    │
   │  /agent/* → worker
   │  /*        → web
   │
   ▼                              ▼
┌──────────┐                  ┌──────────┐
│  Web     │                  │  Agent   │
│  Server  │                  │  Worker  │
│ :8001    │                  │ :18080   │
└──────────┘                  └──────────┘
   tmp/main                      tmp/agent-worker
   cmd/api                       cmd/agent-worker
   hot-reload OK                 lifecycle propio
```

**Step 1 (este commit) entrega:**
- `cmd/bff/main.go` (~50 líneas, `httputil.NewSingleHostReverseProxy`).
- `cmd/agent-worker/main.go` (~50 líneas, sólo `/agent/healthz` hoy).
- `scripts/run-all.sh` para arrancar/parar los 3 procesos.
- Unit tests para el routing del BFF.
- Unit tests para el contrato `/agent/healthz` del worker.
- Una *verificación manual* con todos los upstreams arriba: el browser
  recibe respuesta desde el agent-worker vía BFF sin que el hot-reload
  del web-server lo perturbe.

**Step 2 (próximo turno) migra los handlers reales de `pkg/agent/http`
al worker:** los endpoints `/agent/sessions/.../prompt`, `/events`,
`/abort`, etc. viven en el worker; el web-server deja de registrarlos.
La migración debe preservar el contrato externo (paths, bodies) para
que el browser no note diferencia.

**Por qué sirve para 12-factor**: cada proceso cumple IX
(disposability). El worker es barato de tirar y respawn; el web-server
es stateless-meta; el BFF nunca cambia. La suma de los tres es el
"servicio" desde la perspectiva del browser.

## 11. Diseño interno del BFF

El BFF tiene un solo trabajo: rutear. Sus invariantes:

| Invariante                                  | Test                            |
| -------------------------------------------- | ------------------------------- |
| `/agent/*` (no `/agents`) va al worker      | `TestIsAgentRoute` con tabla    |
| `/*` va al web-server                        | `TestBFFRouteWebAndAgent`       |
| Si el upstream está caído, devuelve 502     | `TestBFFReturns502WhenAgentDown`|
| El BFF nunca tiene lógica de negocio        | `bffHandler` es ~10 líneas      |
| El BFF puede arrancarse sin estado           | `bffHandler` no lee state      |

NOTA: en step 2, los handlers reales del agente irán al worker. El
`bffHandler` se mantiene igual; sólo cambia el handler del worker.

## 12. Diseño interno del agent-worker

El agent-worker es un `http.Server` shrunk (sin middleware, sin
templates, sin DB). Por ahora sólo expone `/agent/healthz`.

| Endpoint        | Step 1                  | Step 2 (próximo turno)             |
| --------------- | ----------------------- | ---------------------------------- |
| `GET /agent/healthz` | 200 `{"status":"alive"}` | idem                            |
| `* /agent/...`       | 501 Not Implemented    | handlers reales migrados           |
| Models               | stdlib `net/http`        | stdlib + AgentService via pkg    |

En step 2 el worker se construye en `pkg/agent/cmd/worker/main.go`. Este
worker ya no necesita un `.air.toml` separado: air mira `pkg/agent/`
por su `include_ext`, así que cubre también el cmd del worker.

## 13. Cómo correr los tres procesos en dev

```sh
# 1. compilar y arrancar
scripts/run-all.sh start

# 2. verificar status (lee pidfiles y hace healthcheck)
scripts/run-all.sh status

# 3. parar limpio
scripts/run-all.sh stop
```

Los pidfiles viven en `tmp/run/{name}.pid`. El BFF se compila como
artefacto estático (`./bin/bff`) y se puede regenerar a mano:

```sh
go build -o ./bin/bff ./cmd/bff
```

NOTA sobre air: air NO mantiene al BFF porque no comparte módulos
runtime con él. Si tocás `cmd/bff/main.go`, recompilás el BFF a mano
y reiniciás sólo ese proceso. Cero hot-reload innecesario.



## 14. Authentication: cada servicio valida JWT contra el IdP (Opción A)

A partir de 2026-07-02, después del feedback de "boilerplate apuntando a
producción", elegimos el modelo estándar de industria en lugar del
"BFF como gate":

- **El BFF es un proxy TONTO.** No toca el header `Authorization`;
  preserva el JWT intacto al forwardear.
- **Cada upstream valida el JWT contra el IdP independientemente.**
  Web-server y agent-worker corren su propio JWTMiddleware contra
  Casdoor (mismos env: `JWKS_URL`, `OIDC_ISSUER`, `JWT_AUDIENCE`).
- **Sin secretos compartidos, sin internal token HMAC.** La auth es
  puramente OIDC estándar.

### Por qué Opción A y no "BFF con internal token"

| Aspecto | Opción A (elegida) | BFF con internal token |
|---|---|---|
| Independencia de procesos | Cada servicio desplegable solo | BFF era single-point-of-failure |
| Estándar industry | ✓ Lo que Casdoor/Keycloak/Auth0 esperan | Custom, fuera de estándar |
| Red de confianza | Cada upstream confía en el JWT firmado por el IdP | Cada upstream confía en HMAC del BFF |
| Latency marginal | IdP JWKS cached; cada servicio ya tenía que tenerlo | HMAC cached |
| Código custom | Sólo el routing del BFF (50 líneas) | BFF valida + emite token, upstreams verifican |
| Rotación secrets | No hay secretos internos | 3 lugares a actualizar si rota `BFF_INTERNAL_SECRET` |

Decisión: para producción escalable, Opción A. El BFF ahora es
muy chico. Si el BFF se cae, los upstreams siguen sirviendo con JWT
válido.

### El flujo

```
   browser
      │
      │ Authorization: Bearer <jwt>
      ▼
 ┌──────────┐
 │   BFF    │  ← TONTO: copy-through reverse proxy.
 │ :8000    │     NO valida nada. NO inyecta nada.
 └──┬────┬──┘
    │    │
    └    │      Authorization: Bearer <jwt>  (SIN modificar)
         │
   /*   │
         ▼
      web-server     ─── valida JWT contra JWKS_URL (Casdoor).
                      Si OK, claims = { email }.
                      Si no, 401.
   /agent/*
         │
         ▼
      agent-worker   ─── valida JWT contra JWKS_URL (Casdoor).
                      Si OK, extrae email y dispatchea al handler.
                      Si no, 401.
```

### Variables de entorno

Cada uno de los 3 servicios puede compartir las mismas env vars de
JWT (JWKS_URL, OIDC_ISSUER, etc.). En la práctica los deployás
iguales y el operador las rota junto al rotar el IdP.

| Env var | Default | Propósito |
| --- | --- | --- |
| `JWKS_URL` | (sin default; el worker refuses boot sin esto) | URL del JWKS del IdP. Para Casdoor: `https://<casdoor>/.well-known/jwks.json`. |
| `JWT_HMAC_SECRET` | (vacío alternativo) | Modo dev/CI: HMAC shared-secret. NO usar en prod. |
| `OIDC_ISSUER` | (sin default) | Si está poblado, BFF/worker exige `iss == $OIDC_ISSUER`. |
| `OIDC_CLIENT_ID` | (sin default) | Identifica al consumer del JWT (aud cuando no hay `aud` explícito). |
| `JWT_AUDIENCE` | (sin default) | Si está poblado, exige claim `aud` matching. |
| `AUTH_DISABLED` | "false" | Si "true", ambos servicios aceptan `X-Dev-Sub` / `X-Dev-Email` para CI/tests. NO en prod. |

### Por qué el BFF no valida nada

- **El BFF es de ~80 líneas, no introduce auth.** Si lo bajás o lo
  reemplazás, los upstreams siguen funcionando idénticamente.
- **El browser puede ir directo al worker o al web-server.** Si el
  BFF está caído, el backend sigue prestando servicio con auth
  estándar. En el pasado, todo dependía del BFF.
- **No hay secretos internos que rotar.** Cero superficie de bug
  custom en el camino crítico de auth.

### Smoke test E2E

```
scripts/smoke-bff.sh test    # arranca + corre aserciones + cleanup
```

Cubre:

1. healthz sin Authorization: BFF pasa, worker responde 200 con JSON.
2. ruta protegida sin Authorization: 401 desde el worker.
3. ruta protegida CON Authorization válido: BFF pasa tal cual, worker
   valida JWT y claims llegan post-auth (status != 401).
4. paths públicos (`/auth/login`, `/favicon.ico`, `/agent/healthz`):
   no requieren auth, no devuelven 401.

### Lo que cambió cuando pasamos a A

Archivos borrados (~520 líneas eliminadas):

- `internal/auth/internal_token/` (HMAC).
- `cmd/bff/internal/auth/` (la JWT validation del BFF).
- `pkg/agent/worker/auth/` (middleware internal token del worker).
- `internal/auth/middleware/internaltoken_test.go` (tests del path X-Internal-Auth).

Archivos modificados:

- `cmd/bff/main.go`: de ~210 líneas (con auth) a ~80 líneas (sólo proxy).
- `cmd/agent-worker/main.go`: usa `authmiddleware.JWTMiddleware` directo
  con `JWKS_URL` o `JWT_HMAC_SECRET`.
- `internal/auth/middleware/middleware.go`: borrada la rama
  `X-Internal-Auth`; ahora `JWTMiddleware` es cookie + JWT, sin el
  atajo de HMAC interno.

### Lo que queda pendiente (cuando pivotemos a xterm + wede)

- `cmd/bff`, `cmd/agent-worker`, `pkg/agent/worker/handlers/` se
  reemplazan con un módulo standalone (`pkg/xterm`) que wede carga
  como extensión o como proceso hermano.
- El BFF deja de ser necesario: wede actúa como gateway + auth host
  del browser.
- La auth en xterm puede reusar `internal/auth/middleware.JWTMiddleware`
  o integrarse directamente con wede.auth. Sin secretos internos que
  coordinar.
- Migrar `internal/auth/internal_token/` a `xterm` si se quiere un
  service-to-service token entre wede y xterm (distinto al JWT del
  usuario). Pero ya no es crítico.

