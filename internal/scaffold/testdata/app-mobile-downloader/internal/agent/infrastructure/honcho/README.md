# honcho

Adapter del bounded context `agent` contra [Honcho](https://honcho.dev)
v3. Implementa `agentapp.MemoryProvider` para inyectar contexto
relevante antes de un prompt (Recall) y persistir al final de un turno
(Remember).

## Estructura

```
honcho/
├── client.go       // wrapper HTTP fino contra api.honcho.dev
├── types.go        // structs request/response del API v3
├── keys.go         // MemoryKey → (workspace, session, agent peer, user peer)
├── adapter.go      // implementa MemoryProvider usando Client
├── client_test.go  // 11 tests con httptest (1 por método + casos error)
└── adapter_test.go // 13 tests de comportamiento (idempotencia, batching, truncado, aislamiento)
```

## Endpoints usados

| Operación | Método | Path |
|---|---|---|
| Workspace upsert | POST | `/v3/workspaces` |
| Peer upsert | POST | `/v3/workspaces/{wid}/peers` |
| Session create con peers | POST | `/v3/workspaces/{wid}/sessions` |
| Peer add a session | POST | `/v3/workspaces/{wid}/sessions/{sid}/peers` |
| Session context (Recall) | GET | `/v3/workspaces/{wid}/sessions/{sid}/context` |
| Messages batch (Remember) | POST | `/v3/workspaces/{wid}/sessions/{sid}/messages` |

## Configuración (env vars)

| Variable | Default | Notas |
|---|---|---|
| `HONCHO_API_KEY` | (sin default) | obligatorio |
| `HONCHO_BASE_URL` | `https://api.honcho.dev` | opcional |
| `HONCHO_WORKSPACE_ID` | `lastmile-agents` | opcional |
| `HONCHO_TOKEN_BUDGET` | `1000` | cap server-side para context |
| `HONCHO_TOP_K` | `8` | conclusions a recuperar por search |
| `HONCHO_RECALL_TIMEOUT_MS` | `2000` | timeout duro del Recall |

## Tests

```sh
go test ./internal/agent/infrastructure/honcho/...
```

Todos los tests corren contra `httptest`. No pegan contra Honcho real.

## Limitaciones connues (Fase B v1)

- **No hay reintentos**: un 5xx del backend hace fallar el flush. Honcho
  sí deduplica semánticamente vía deriver, pero reintentos manuales
  duplican mensajes. Si hace falta robustez, agregar retry con
  backoff exponencial sólo para 5xx.
- **No hay cache de IDs**: cada `EnsurePeers` repite los 4 calls HTTP.
  Honcho los hace upsert idempotente, así que es barato, pero si el
  costo es problema, agregar un `sync.Map[MemoryKey]resolvedIDs` con
  TTL.
- **Sin SDK oficial de Go**: Honcho solo tiene SDKs Python y
  TypeScript. Este módulo es HTTP plano contra
  `https://api.honcho.dev/v3`. Si Honcho rompe compatibilidad de API,
  el módulo se entera por errores de decode/tests fallidos, no en
  build.

## Próximas fases

- **Fase C** (`internal/agent/application/manager.go`): inyectar
  Recall/Remember en el prompt path. Slot marker para no repetir
  steer en cada Prompt del mismo turn. Flush al `turn_end`/`agent_end`.
- **Fase D** (`internal/agent/infrastructure/pirpc/process.go`):
  filtrar `.pi/extensions/honcho` del spawn cuando `HONCHO_ENABLED=true`
  (forward-compat; hoy no hay tal extensión).
