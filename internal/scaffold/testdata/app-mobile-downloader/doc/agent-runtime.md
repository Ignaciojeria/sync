# Runtime del agente pi en `gitinittest5`

> Documento vivo para entender qué partes del módulo `internal/agent`
> sobreviven a un fork del repo sin intervención y qué partes requieren
> ajustes al migrar a otro proyecto (boilerplate). También cubre los
> puntos pendientes para llegar a 12-factor compliance estricto.

> **Nota 2026-07-08:** el modo activo del proyecto es **app única**
> en `cmd/api`. Este doc describe únicamente el flujo actual.

> **Nota 2026-07-20 (multi-agente):** las reglas específicas del
> agente (capas, sandbox, opt-out, runtime embebido) viven ahora en
> `agents/develop/AGENTS.md`. El `.pi/` del agente se sembró desde
> la raíz a `agents/develop/.pi/` como preparación para multi-
> agente. Cuando aparezcan nuevos agentes (`reviewer`, `docs`,
> etc.), cada uno vivirá bajo `agents/<id>/` con su propio `.pi/`
> y `AGENTS.md`. El registro está en
> `internal/agent/application/registry.go` y se invoca via
> `AgentService.CreateSession(input.AgentID)`; vacío resuelve al
> default (`"develop"`).

### Sandbox y workspaces por agente (2026-07-20)

A partir de la separación multi-agente, el sandbox de cada sesión
se siembra desde `agents/<agentID>/` (no desde la raíz del repo):

- `.pi/` se copia desde `agents/<agentID>/.pi/` (era `./.pi/`
  antes).
- El `AGENTS.md` del workspace del agente se copia al sandbox
  para que pi lo lea como reglas del agente.
- El `AGENTS.md` **raíz** del repo **no** se siembra dentro del
  sandbox — es la fuente de reglas para humanos/IA que abren el
  repo desde su editor, no para el agente embebido.

Los sandboxes existentes en `tmp/agent-work/` siguen funcionando
porque ya tienen su `.pi` copiado adentro; este card **no
migra** sesiones existentes. Si una sesión legada quiere
actualizar su `.pi`, hay que borrar el sandbox y crear sesión
nueva (decisión consciente: las sesiones en curso podrían tener
configuración modificada que no queremos pisar).

## 0. Pool de runtimes (cambio del 2026-07-03 — baja RAM drástica)
**Antes**: cada chat usado mantenía 1 proceso `pi` vivo
(`map[sessionID]Runtime` en `internal/agent/application/manager.go`).
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
path documentado en `internal/agent/application/manager.go` §
`maybeEvictForNewSlot`.

## 1. Lo verificado el 2026-07-02

Comprobado empíricamente con `pi --mode rpc --provider anthropic --model
claude-sonnet` desde una VM con `air` activo:

- El agente pi tiene `read`/`bash`/`write`/`edit` tools y **puede editar
  archivos del repo sin pedir confirmación**: en la prueba le pedimos
  que agregara una línea al top de
  `internal/agent/infrastructure/pirpc/runner.go` y lo hizo en ~3 s.
- Editar un archivo bajo `internal/agent/` **disparó el watcher de air
  → rebuild → reinicio del servidor Go**, dejando el proceso `pi`
  huérfano. Esto es lo que el usuario sintió como "se cuelga y deja de
  funcionar".
- El fix aplicado (`internal/agent/infrastructure/pirpc/sandbox.go`)
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
`gitinittest5/<paquete>`. Al forkear a un proyecto nuevo hay que:

```sh
# desde la raíz del fork
go mod edit -module github.com/mi-org/mi-proyecto
rg -l "gitinittest5" | xargs sed -i 's|gitinittest5|mi-proyecto|g'
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
rg -l "gitinittest5" | xargs sed -i 's|gitinittest5|<nuevo-path>|g'
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
  `internal/agent/infrastructure/pirpc/sandbox.go`
- Proceso afectado:
  `internal/agent/infrastructure/pirpc/process.go`
- Página que arranca el agente con CWD vacío:
  `internal/agent/http/page.go`
- Configuración de air que debe sobrevivir al fork:
  `.air.toml` (línea `exclude_dir`)
- Plan original del módulo:
  `AGENT_IMPLEMENTATION_PLAN.md`

## 9. Layering módulo y opt-out

El agente vive en `internal/agent/`, igual que auth, editor, home, quality
y scheduler. Sigue las mismas capas (`application/`, `http/`,
`infrastructure/`, `ui/`) y se enciende/apaga con la flag `AGENT_ENABLED`
sin tocar el resto del wiring. La estructura:

```
internal/agent/
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
Declarada en `internal/agent/application/manager.go`:

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

Hoy `internal/agent/http` y `internal/agent/application` todavía importan tipos
del host (`internal/shared/server`, `internal/ui/layout`,
`internal/auth/middleware`). El path de imports llega via go's `internal`
rule porque `internal/agent` está dentro del mismo módulo.

Para llegar al "nivel 2" del análisis original (módulo Go separado
con `go.work`, dependencias invertidas vía interfaces en lugar de
imports directos), falta:
1. Mover `internal/shared/server`, `internal/ui/layout`,
   `internal/auth/middleware` a paths non-`internal`.
2. Definir en `internal/agent` interfaces `HostRouter`, `HostMiddlewareFunc`,
   `LayoutResolver` (análogas a `AgentService`).
3. `internal/agent/http` consume las interfaces, no los tipos concretos del
   host.
4. `cmd/api/main.go` provee adapters (fuego → HostRouter, etc.).

Esto queda como TODO; no hace falta para que el boilerplate funcione
y se puede hacer incrementalmente sin romper llamadas existentes.

