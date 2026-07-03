# BFF — invariantes de routing y Auth

El BFF (`cmd/bff/main.go`) es un proxy inverso tonto: ~50 líneas, sin
lógica de negocio. Su trabajo es mirar el path y forwardear el request
tal cual al upstream que corresponda. Los invariantes que defendía el
`scripts/smoke-bff.sh` (ya erradicado) y que cualquier cambio al BFF
tiene que respetar:

## Topología

```
   browser
      │
      ▼
   BFF  :8000  (cmd/bff)              ← este proxy
      ├─►  worker :18080  (cmd/agent-worker)   para /agent/<algo> (API + SSE)
      └─►  web    :8001  (cmd/api)            para /agent (UI) y todo lo demás
```

**La frontera**:

| Path                      | Upstream | Por qué                           |
|---------------------------|----------|-----------------------------------|
| `/agent`                  | web      | UI (templ.Page) registrada por `pkg/agent/http.Register` |
| `/agent/<cualquier cosa>` | worker   | API JSON + SSE + healthz          |
| `/agents` (plural)        | web      | No es del agente — fuera del scope |
| todo lo demás             | web      | Auth flow, editor, etc.           |

Reglas en código, todas en `cmd/bff/main.go:isAgentRoute`:

```go
// Sólo lo que arranca con "/agent/" (con barra final) va al worker.
// "/agent" exacto va al web, junto con "/agents" plural y todo lo demás.
return strings.HasPrefix(path, "/agent/")
```

## Invariantes (Opción A — validación directa contra el IdP)

1. **Worker liveness sin auth.** `GET /agent/healthz` responde `200`
   con `{"status":"alive"}` desde el BFF, sin header `Authorization`.
   El handler es público a propósito: es una liveness probe.

2. **`GET /agent` (sin trailing `/`) es la UI HTML.** La sirve el
   **web-server** vía `templ.Page` (`pkg/agent/ui/page.templ`), no el
   worker. El BFF la deja pasar al web. Cuando el browser llega sin
   sesión, el middleware del web lo redirige (302) a `/auth/login`.

   Antes la BFF capturaba `/agent` exacto y lo mandaba al worker. El
   worker no tenía handler → 404 (o, después de un intento de fill-in,
   un JSON con la lista de endpoints). Una de las dos: ninguna era
   útil. Ahora la frontera es `/agent/<x>` → worker, `/agent` pelado
   → web.

3. **Rutas de agent SIN Authorization → 401.** Cualquier `/agent/<x>`
   sin JWT válido devuelve `401` desde el BFF. El BFF no valida nada:
   si el worker responde 401, el BFF propaga.

4. **Rutas de agent CON Authorization válido → llega al worker.**
   El BFF es Opción A: deja pasar el header `Authorization` tal cual
   al upstream. El worker corre su propio `JWTMiddleware` validando
   contra `JWKS_URL` (Casdoor) — HMAC es opt-in vía `JWT_HMAC_SECRET`
   y sólo se usa en dev/CI. Si el JWT está bien firmado y los claims
   matchean issuer/audience, el request sigue hasta el handler. Si el
   session id no existe, el worker devuelve `404`; cualquier cosa que
   no sea `401` significa que la auth pasó.

5. **Rutas públicas no requieren auth y van a `web`.** `/auth/login`,
   `/auth/callback`, `/auth/logout`, `/manifest.json`, `/favicon.ico`
   se forwardean al upstream `web` sin auth. La respuesta no debe ser
   `401` desde el BFF. Si el web está caído, el BFF devuelve `502`
   (aceptable — el operador ve el Web upstream caído).

## Failure modes que el BFF tiene que propagar sin mentir

| Condición                          | Respuesta esperada | Por qué                    |
|------------------------------------|--------------------|----------------------------|
| Worker upstream muerto, `/agent/<x>` | `502`            | Proxy reporta el dial fail |
| Web upstream muerto, `/*` (incluye `/agent` UI) | `502` | Idem |
| Worker responde 401                | `401`              | Auth vive en el worker     |
| Worker responde 404                | `404`              | Session not found (auth pasó) |
| Web responde 401 (sesión inválida) | redirige a `/auth/login` o 401 según Accept | Middleware del web |

## Cobertura

- `cmd/bff/main_test.go` cubre los invariantes 3/5, el caso
  502-on-agent-down, y la **frontera `TestIsAgentRoute`** (que valida
  la tabla de la sección Topología: `/agent` → web, `/agent/<x>` →
  worker, `/agents` plural → web). Todo con `httptest`, sin procesos
  externos, sin pids, sin cleanup.
- `pkg/agent/application/manager_test.go` y `pkg/agent/infrastructure/pirpc/{sandbox,runner}_test.go` cubren el lado del worker.
- Para validar el invariante 2 end-to-end: login en
  `https://<host>/auth/login`, luego `GET https://<host>/agent` debe
  devolver HTML (templ.Page con el chat), no JSON. Verificable
  también desde la VM con `curl -sb cookies.txt http://127.0.0.1:8000/agent`.
- El invariante 4 (worker acepta JWT firmado por Casdoor y rechaza
  HMAC en este modo) se valida con `curl -H "Authorization: Bearer
  <jwt-casdoor>" http://127.0.0.1:8000/agent/sessions/test/get`. Un
  JWT HS256 firmado con el viejo HMAC secret **debe** ser rechazado
  (cualquier ruta `/agent/<x>` → 401).

## Homologación auth (worker ↔ web)

Desde 2026-07-02 ambos procesos validan JWT contra el mismo JWKS del
IdP Casdoor. La política es:

- **Default (dev y prod)**: `JWKS_URL` se deduce de `OIDC_JWKS_URI` en
  el `.env` del proyecto. Si no hay `.env`, ambos procesos fallan loud
  al arrancar (`loadKeyfunc: ni JWKS_URL ni JWT_HMAC_SECRET configurados`).
- **HMAC**: queda opt-in vía `JWT_HMAC_SECRET` explícito en el shell
  del operador. Útil para CI aislado sin red al IdP. El worker loguea
  `HMAC secret mode (NO usar en prod)` para que quede registro.
- **`scripts/run-all.sh`** ya no inyecta un secret HMAC por default.
  Lee `OIDC_JWKS_URI`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `JWT_AUDIENCE`
  del `.env` con un helper tolerante a claves ausentes (`grep` con
  error silenciado). Los devs que quieran HMAC deben exportarlo ellos
  mismos.

Con esto, el invariante "el BFF se cae, los upstreams siguen sirviendo
con JWT válido" (`AGENTS.md §14`) se cumple de verdad: tanto el web
como el worker validan contra el mismo IdP, sin secretos compartidos
internos ni via BFF.

## Reposicionamiento histórico

`scripts/smoke-bff.sh` validaba los invariantes originales a costa de
levantar tres procesos en puertos distintos y hacer `pkill -f` agresivo
en el cleanup. Se erradicó el 2026-07-02 porque rompía el estado del
orquestador (`run-all.sh`) cada vez que se corría. La cobertura vive
en Go tests desde entonces, y los invariantes quedan documentados
acá para review humana.
