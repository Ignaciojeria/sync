# Backlog como source of truth

> **Regla del proyecto:** el backlog en `internal/backlog/board/`
> (servido en `/backlog`) es la **única fuente de verdad** para
> validar y ejecutar trabajo. Cualquier plan, RFC o lista de TODOs
> que no esté allí no cuenta.

## Por qué

- El repo ya tiene el módulo `internal/backlog` con persistencia en
  disco, kanban (`backlog` → `todo` → `in_progress` → `done`) y
  prioridad (`P0..P3`).
- Sin un store único, los planes viven en chats, docs sueltos o
  commits y se desincronizan. El backlog los expone a la UI, al CLI y
  al agente con el mismo formato.

## Cómo se accede

### Desde la UI (humanos)

```
http://localhost:<puerto>/backlog
```

Las operaciones CRUD están en `/backlog/cards/*` (HTMX).

### Desde el CLI (agente + scripts)

El wrapper vive en `tmp/backlog-cli/` y reusa `backlogapp.Manager` +
`backlogdisk.CardStore` (los **mismos** paquetes que el server). Eso
garantiza que el formato de las cards sea idéntico en ambos lados.

```bash
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli list
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli next
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli show <id>
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli create -t "Título" [-d DESC] [-s STATUS] [-p P0]
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli move <id> <status>
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli priority <id> <P0..P3>
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli delete <id>
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli seed     # siembra plan developer-teams
```

`BACKLOG_DIR` default = `internal/backlog/board`. Mismo path que el
server (a menos que el server se arranque con `BACKLOG_DIR` distinto).

## Reglas de uso

1. **Una card por unidad de trabajo.** Si una card tiene más de un
   objetivo, partirla.
2. **Prioridad explícita.** P0 = bloqueante, P1 = importante,
   P2 = nice-to-have, P3 = someday.
3. **Estado refleja realidad.**
   - `backlog` = identificado pero no comprometido
   - `todo` = comprometido, listo para empezar
   - `in_progress` = alguien lo está haciendo (idealmente solo una
     card por agente en este estado)
   - `done` = terminado y verificado
4. **Idempotencia.** `seed` deduplica por título, no duplica si ya
   existe. `create` siempre crea con ID nuevo.
5. **Mover al hablar.** Si decís "voy a empezar X", primero
   `move X todo` y luego `move X in_progress` antes de tocar código.

## Flujo recomendado para el agente

```bash
# 1. ¿Qué hago ahora?
NEXT=$(BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli next)
ID=$(echo "$NEXT" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')

# 2. Promover a in_progress
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli move "$ID" in_progress

# 3. Hacer el trabajo...

# 4. Cerrar
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli move "$ID" done
```

## Plan vigente

Las cards del plan **Developer Teams** se siembran con:

```bash
BACKLOG_DIR=internal/backlog/board go run ./tmp/backlog-cli seed
```

Son idempotentes. Para empezar desde cero:

```bash
rm -rf internal/backlog/board && go run ./tmp/backlog-cli seed
```

El detalle del plan vive en `doc/agent-teams-plan.md`; las cards son
la vista operativa.

## Por qué no usar HTTP para el agente

- El server puede estar con auth OIDC real (no dev), lo que bloquea
  cualquier llamada del agente sin pasar por login.
- HTTP añade JSON parsing, headers y una superficie de fallo más
  amplia. El CLI importa directamente el paquete del módulo: cero
  serialización intermedia, cero auth.
- El CLI es **mismo binario que el server en cuanto a lógica de
  dominio**: ambos usan `backlogapp.Manager`. Lo que ves con `list`
  es exactamente lo que ve la UI.

## Cuando NO usar el backlog

- Notas exploratorias o brainstorms → `doc/` con prefijo `rfc-` o
  `idea-`.
- Bugs durante una sesión → si son triviales, se arreglan y se
  menciona en el commit. Si no, se crea una card.
- Trabajo < 15 min → commit directo + mensaje claro. No todo merece
  una card.