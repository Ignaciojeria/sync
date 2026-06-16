# Estructura del Proyecto

**Single-binary** (`cmd/api`) con módulos separados por contexto (`app`, `editor`, `dev`). Cada módulo tiene exactamente la misma forma: `application/` + `http/` + `ui/`. Todos son autocontenidos.

```text
app-mobile-downloader/
├── cmd/
│   └── api/
│       ├── main.go                 # Punto de entrada. Importa módulos vía IoC.
│       └── main_test.go
│
├── internal/
│   ├── app/                        # Módulo: negocio + auth
│   │   ├── application/            # Vacío (placeholder)
│   │   ├── http/
│   │   │   ├── hello.go            # GET|POST /
│   │   │   ├── auth_login.go       # GET /auth/login, /auth/login/google
│   │   │   ├── auth_callback.go    # GET /auth/callback, /auth/logout
│   │   │   ├── auth_login_test.go
│   │   │   └── hello_test.go
│   │   └── ui/                     # Templates de app (placeholder)
│   │
│   ├── editor/                     # Módulo: proxy/editor
│   │   ├── application/            # Vacío (placeholder)
│   │   ├── http/
│   │   │   ├── proxy.go            # Proxy /editor/*, /assets/*, /api/*, etc.
│   │   │   └── proxy_test.go
│   │   └── ui/                     # Templates de editor (placeholder)
│   │
│   ├── dev/                        # Módulo: herramientas de desarrollo
│   │   ├── application/
│   │   │   └── test_report/
│   │   │       ├── runner.go       # Lógica de ejecución de tests + cobertura
│   │   │       └── runner_test.go
│   │   ├── http/
│   │   │   ├── test_report_page.go     # GET /report/tests
│   │   │   ├── test_report_run.go      # POST /report/tests/run
│   │   │   ├── test_report_coverage.go # GET /report/tests/coverage.html
│   │   │   ├── test_report_support.go  # Helpers de renderizado
│   │   │   └── test_report_test.go
│   │   └── ui/                     # Templates + estado + renderizado
│   │       ├── state.go              # TestRunState + helpers
│   │       ├── state_test.go
│   │       ├── render.go             # RenderResultAndDashboard
│   │       ├── page.templ            # Página completa /report/tests
│   │       ├── page_templ.go
│   │       ├── fragments.templ       # Fragments HTMX: DashboardStats, TestResult
│   │       └── fragments_templ.go
│   │
│   └── shared/                     # Cross-cutting concerns
│       ├── access/
│       │   ├── allowlist.go        # Allowed emails (editor/app)
│       │   └── allowlist_test.go
│       ├── configuration/
│       │   ├── conf.go             # Estructura de env vars
│       │   ├── conf_test.go
│       │   ├── parse.go            # Parseo de env vars
│       │   └── parse_test.go
│       ├── claims.go               # Helpers: FirstStringClaim, FirstNonEmpty
│       ├── jwks/
│       │   ├── new.go              # Fetch de JWKS
│       │   └── new_test.go
│       ├── server/
│       │   ├── new.go              # Setup de Fuego
│       │   ├── new_test.go
│       │   ├── doc/
│       │   │   └── openapi.json
│       │   └── middleware/
│       │       ├── middleware.go   # JWT, session cookie, auth paths
│       │       └── middleware_test.go
│       └── infrastructure/
│           ├── postgresql/
│           │   ├── connection.go
│           │   ├── connection_test.go
│           │   ├── session_store.go      # Repositorio de sesiones
│           │   ├── session_store_test.go
│           │   └── migrations/
│           │       ├── 000001_create_users_and_sessions.up.sql
│           │       └── 000001_create_users_and_sessions.down.sql
│           └── test/
│               ├── support.go      # Soporte para tests (infraestructura)
│               └── support_test.go
│
├── internal/ui/                    # Componentes UI compartidos
│   └── layout/
│       ├── layout.templ            # Layout base (@layout.Layout)
│       └── layout_templ.go
│
├── go.mod
├── go.sum
├── Makefile
├── docker-compose.yml              # Postgres + IDP OIDC
├── .env
└── AGENTS.md
```

---

## Convenciones

### Handlers
- **Un archivo por handler**. Ejemplo: `auth_login.go`, `auth_callback.go`, `proxy.go`.
- Viven en `internal/<contexto>/http/`.
- Se registran vía IoC con `ioc.Register(nombreHandler)`.

### Templates
- **Templates por contexto**: cada módulo tiene su propio `ui/` con `package ui`.
- **Feature pages**: `page.templ` — páginas completas que usan `@layout.Layout("Título")`.
- **Feature fragments**: `fragments.templ` — partials/fragments HTMX del mismo feature.
- **Modelos compartidos**: `state.go` en `ui/` contiene `TestRunState` y helpers.
- **Renderizado**: `render.go` en `ui/` contiene `RenderResultAndDashboard()`.
- **Layout global**: `internal/ui/layout/layout.templ` con `package layout`.
- **Componentes UI**: `internal/ui/<componente>/` como packages independientes (ej: `button/`, `navbar/`). Solo se crean cuando se usan.

### Auth
- No hay `cmd/auth` separado ni `internal/shared/auth/`.
- El flujo OIDC está en los handlers (`auth_login.go`, `auth_callback.go`) y en `server/middleware/middleware.go`.
- El middleware valida JWT, maneja sesiones en PostgreSQL y controla rutas públicas/editor/app.
- Cualquier IDP compatible OIDC funciona cambiando solo `OIDC_ISSUER_URL` en config.

### IoC
- Usa `github.com/Ignaciojeria/ioc`.
- Los paquetes se auto-registran importando con `_` en `cmd/api/main.go`.
- No hay archivo `ioc.go` explícito; el registro es por side-effect en cada paquete.

### Helpers compartidos
- `internal/shared/claims.go` contiene `FirstStringClaim` y `FirstNonEmpty`.
- Evita duplicación de código entre handlers y middleware.

---

## Principio clave

**Cada contexto es autocontenido y sigue la misma plantilla.**

Cuando trabajas en `dev`, todo está en `internal/dev/`:
- `application/` — lógica de negocio
- `http/` — handlers
- `ui/` — templates + estado + renderizado

Lo mismo aplica para `app` y `editor`. No necesitas saltar a otra carpeta del proyecto.

Para un agente, esto significa que puede abrir solo `internal/dev/` y tiene todo lo necesario:

```
module: dev
http:
    test_report_page.go
    test_report_run.go
application:
    runner.go
ui:
    page.templ
    fragments.templ
    state.go
    render.go
```
