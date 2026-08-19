# AGENTS — Agente `develop`

> Reglas específicas del agente `develop`. El `AGENTS.md` raíz del
> proyecto contiene las reglas globales (frontend, design system,
> auth, tests, skills). Este archivo **no** duplica esas reglas: se
> enfoca en cómo opera este agente dentro del proyecto.

Este archivo se copia a cada sandbox (`tmp/agent-work/<sessionID>/`)
al iniciar una sesión, junto con el `.pi/` desde
`agents/develop/.pi/`. Si modificás reglas acá, las sesiones nuevas
las ven en su siguiente spawn. Las sesiones existentes siguen con
la copia vieja hasta que se recreen.

## Capas del módulo `internal/agent/`

```
internal/agent/
├── application/         ← AgentService (interfaz pública) + Manager (impl)
│                          + registry.go (lista de agentes disponibles)
├── http/                ← handlers /agent/* (consumen AgentService)
├── infrastructure/
│   ├── pirpc/           ← spawn de pi, sandbox CWD, prompt timeout
│   ├── disk/            ← session store persistente en AGENT_SESSION_DIR
│   └── memory/          ← session store en RAM (fallback)
└── ui/                  ← templates templ del chat
```

## Reglas del módulo

1. El host habla con el agente sólo vía `agentapp.AgentService`. Nada de
   tocar `*agentapp.Manager` desde fuera del paquete.
2. El sandbox CWD se resuelve solo en `pirpc.resolveCWD`. Si el caller
   pasa un CWD vacío o `.`, el runner lo redirige a
   `tmp/agent-work/<sessionID>/`.
3. El `.air.toml` debe seguir excluyendo `tmp/` para que las ediciones
   del agente dentro del sandbox no disparen rebuilds.
4. Para apagar el agente sin tocar el código: `AGENT_ENABLED=false`.
   El host omite los endpoints `/agent/*` y no levanta `pi`.

## Sandbox y multi-agente

Cada sesión del agente `develop` corre en su propio sandbox aislado
bajo `tmp/agent-work/<sessionID>/`. Al iniciar:

1. Se siembra `.pi/` desde `agents/develop/.pi/` (idempotente: no
   pisa configuraciones modificadas dentro del sandbox).
2. Se copia este `AGENTS.md` al sandbox para que pi lo lea como
   reglas del agente.
3. El `AGENTS.md` **raíz** ya NO se siembra dentro del sandbox — es
   la fuente de reglas para el humano/IA que abre el repo, no para
   el agente embebido.

### Registry de agentes

El archivo `internal/agent/application/registry.go` define los
agentes disponibles. Hoy hay uno solo:

```go
var DefaultAgents = []AgentDescriptor{
    {ID: "develop", Label: "Develop", Default: true},
}
```

Cuando lleguen más agentes (`reviewer`, `docs`, etc.), cada uno
vivirá bajo `agents/<id>/` con su propio `.pi/` y `AGENTS.md`. El
caller puede pasar `AgentID` en `CreateSessionInput` para elegir;
si viene vacío, se resuelve a `"develop"` (el `Default` del
registry).

### Opt-out limpio

Si un proyecto derivado no quiere agente, basta con:

```sh
# al ejecutar el servidor
AGENT_ENABLED=false ./bin/server
```

Y si quieren sacar el wiring del PATH:

```sh
# eliminar el bloque "agent" en cmd/api/main.go:
# - los imports de internal/agent
# - la llamada a registerAgent(s, hooks, newAgentDeps())
```

No hay dependencias del agente que el resto del código acuse de
recibo.

## Modo runtime actual: app única (`cmd/api`)

El proyecto corre ahora como **una sola app** en dev:

```
browser
   ↓
cmd/api (:8001)
   ├─ /agent
   ├─ /agent/auth
   └─ /agent/sessions/*
```

### Reglas

1. El entrypoint activo es `cmd/api`.
2. El agente corre embebido en el mismo proceso vía `agenthttp.RegisterAllLegacy(...)`.
3. `air` recompila `cmd/api` y deja los cambios visibles de una vez.
4. `cmd/bff` y `cmd/agent-worker` fueron eliminados; cualquier referencia vieja es histórica.

### Arranque para dev

```sh
# hot-reload normal
air

# o sin air
bash ./scripts/run-api.sh start
bash ./scripts/run-api.sh status
bash ./scripts/run-api.sh stop
```

### Nota histórica

La documentación de 3 procesos se mantiene solo como referencia histórica en
`doc/agent-runtime.md` y debe considerarse stale salvo que se indique lo
contrario.
