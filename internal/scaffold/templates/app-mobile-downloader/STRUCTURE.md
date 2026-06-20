# Estructura del Proyecto

> **Archivo generado automáticamente.** Ejecutar `scripts/generate-structure.sh` para regenerar.
> **No editar manualmente.**

```
AGENTS.md
STRUCTURE.md
  generate-structure.sh
    main.go
    main_test.go
go.mod
    runner.go
    runner_test.go
    test_report_page.go
    test_report_page_test.go
    test_report_run.go
    test_report_run_test.go
    test_report_coverage.go
    test_report_coverage_test.go
    test_report_support.go
    test_report_test.go
    test_report_support_test.go
    hello.go
    hello_test.go
    callback.go
    callback_test.go
    login_google.go
    login_page.go
    login_test.go
    logout.go
    static.go
    support.go
    proxy.go
    proxy_test.go
    view.go
    view_test.go
    editor_view.templ
    editor_view_templ.go
    home.templ
    home_templ.go
    page.templ
    page_templ.go
    fragments.templ
    fragments_templ.go
    state.go
    state_test.go
    render.go
    render_test.go
    identity.go
    oidc_callback.go
    oidc_login.go
    session.go
    state.go
    session_repository.go
    session_repository_test.go
    middleware.go
    middleware_test.go
    login.templ
    login_templ.go
    layout.templ
    layout_templ.go
    layout_with_nav.templ
    layout_with_nav_templ.go
    sidenav.templ
    sidenav_templ.go
    types.go
    render.go
    allowlist.go
    allowlist_test.go
    claims.go
    claims_test.go
    conf.go
    conf_test.go
    parse.go
    parse_test.go
    connection.go
    connection_test.go
    session_store.go
    session_store_test.go
    migrations/
      000001_create_users_and_sessions.up.sql
      000001_create_users_and_sessions.down.sql
    new.go
    new_test.go
    new.go
    new_test.go
    support.go
    support_test.go
```

---

## Convenciones de estructura

- Cada módulo de negocio vive en `internal/<modulo>/` con sus capas: `application`, `http`, `infrastructure`, `ui`.
- Código compartido: `internal/shared/` (config, auth, server, infra).
- Punto de entrada: `cmd/api/main.go`.
- Plantillas: `internal/<modulo>/ui/` o `internal/ui/layout/`.
- Tests: junto al código (`*_test.go`).
- Skills: `.agents/skills/`.
- Scripts: `scripts/` para automatización (ej. `generate-structure.sh`).

## Módulos (Bounded Contexts)

- **`auth/`** — Autenticación OIDC, sesiones, JWT middleware
- **`editor/`** — Proxy al editor upstream
- **`home/`** — Página de inicio (landing)
- **`quality/`** — Reporte de tests, cobertura, calidad
- **`scheduler/`** — Configuración y ejecución de jobs (placeholder)

## Layouts compartidos

- `internal/ui/layout/layout.templ` — Layout base sin navegación
- `internal/ui/layout/layout_with_nav.templ` — Layout con drawer + sidenav
- `internal/ui/layout/sidenav.templ` — Menú lateral (Inicio, Quality, Scheduler, Editor, Auth)
- `internal/ui/layout/types.go` — `NavigationContext` construido desde request
- `internal/ui/layout/render.go` — `RenderPage(c, title, content)` helper
