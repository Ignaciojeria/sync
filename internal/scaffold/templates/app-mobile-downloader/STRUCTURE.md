# Estructura del Proyecto

> **Archivo generado automáticamente.** Ejecutar `scripts/generate-structure.sh` para regenerar.
> **No editar manualmente.**

```
.
├── .air.toml
├── AGENTS.md
├── STRUCTURE.md
├── cmd/
│   └── api/
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── auth/
│   │   ├── application/
│   │   │   ├── identity.go
│   │   │   ├── oidc_callback.go
│   │   │   ├── oidc_login.go
│   │   │   ├── session.go
│   │   │   └── state.go
│   │   ├── http/
│   │   │   ├── callback.go
│   │   │   ├── login_google.go
│   │   │   ├── login_page.go
│   │   │   ├── logout.go
│   │   │   ├── static.go
│   │   │   └── support.go
│   │   ├── infrastructure/
│   │   │   └── postgresql/
│   │   │       ├── session_repository.go
│   │   │       └── session_repository_test.go
│   │   ├── middleware/
│   │   │   ├── middleware.go
│   │   │   └── middleware_test.go
│   │   └── ui/
│   │       ├── login.templ
│   │       └── login_templ.go
│   ├── editor/
│   │   ├── application/
│   │   ├── http/
│   │   │   ├── proxy.go
│   │   │   ├── proxy_test.go
│   │   │   └── view.go
│   │   └── ui/
│   │       └── editor_view.templ
│   ├── home/
│   │   ├── http/
│   │   │   └── hello.go
│   │   └── ui/
│   │       ├── home.templ
│   │       └── home_templ.go
│   ├── quality/
│   │   ├── application/
│   │   │   └── test_report/
│   │   │       ├── runner.go
│   │   │       └── runner_test.go
│   │   ├── http/
│   │   │   ├── test_report_coverage.go
│   │   │   ├── test_report_coverage_test.go
│   │   │   ├── test_report_page.go
│   │   │   ├── test_report_page_test.go
│   │   │   ├── test_report_run.go
│   │   │   ├── test_report_run_test.go
│   │   │   ├── test_report_support.go
│   │   │   └── test_report_test.go
│   │   └── ui/
│   │       ├── fragments.templ
│   │       ├── fragments_templ.go
│   │       ├── page.templ
│   │       ├── page_templ.go
│   │       ├── render.go
│   │       ├── render_test.go
│   │       ├── state.go
│   │       └── state_test.go
│   ├── scheduler/
│   │   ├── application/
│   │   │   ├── http_client.go
│   │   │   ├── job_config.go
│   │   │   └── job_runner.go
│   │   ├── http/
│   │   │   ├── jobs.go
│   │   │   └── scheduler.go
│   │   ├── infrastructure/
│   │   │   └── postgresql/
│   │   │       └── job_repository.go
│   │   └── ui/
│   │       └── page.templ
│   ├── shared/
│   │   ├── claims.go
│   │   ├── claims_test.go
│   │   ├── access/
│   │   │   ├── allowlist.go
│   │   │   └── allowlist_test.go
│   │   ├── configuration/
│   │   │   ├── conf.go
│   │   │   ├── conf_test.go
│   │   │   ├── parse.go
│   │   │   └── parse_test.go
│   │   ├── infrastructure/
│   │   │   ├── postgresql/
│   │   │   │   ├── connection.go
│   │   │   │   ├── connection_test.go
│   │   │   │   └── migrations/
│   │   │   │       ├── 000001_create_users_and_sessions.down.sql
│   │   │   │       ├── 000001_create_users_and_sessions.up.sql
│   │   │   │       ├── 000002_create_job_configs_and_logs.down.sql
│   │   │   │       └── 000002_create_job_configs_and_logs.up.sql
│   │   │   └── test/
│   │   │       ├── support.go
│   │   │       └── support_test.go
│   │   ├── jwks/
│   │   │   ├── new.go
│   │   │   └── new_test.go
│   │   └── server/
│   │       ├── new.go
│   │       └── new_test.go
│   └── ui/
│       └── layout/
│           ├── layout.templ
│           ├── layout_templ.go
│           ├── layout_with_nav.templ
│           ├── layout_with_nav_templ.go
│           ├── render.go
│           ├── sidenav.templ
│           ├── sidenav_templ.go
│           └── types.go
└── scripts/
    ├── _tree_generator.py
    ├── generate-structure.sh
    └── structure.config.toml
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
