# Estructura del Proyecto

> **Archivo generado automáticamente.** Ejecutar `scripts/generate-structure.sh` para regenerar.
> **No editar manualmente.**

```
.
├── .air.toml
├── AGENTS.md
├── STRUCTURE.md
├── go.mod
├── cmd/
│   └── api/
│       ├── main.go
│       └── main_test.go
├── internal/
│   ├── auth/
│   │   ├── application/
│   │   │   ├── identity.go
│   │   │   ├── identity_test.go
│   │   │   ├── oidc_callback.go
│   │   │   ├── oidc_callback_test.go
│   │   │   ├── oidc_login.go
│   │   │   ├── oidc_login_test.go
│   │   │   ├── session.go
│   │   │   ├── state.go
│   │   │   └── state_test.go
│   │   ├── http/
│   │   │   ├── callback.go
│   │   │   ├── callback_test.go
│   │   │   ├── login_google.go
│   │   │   ├── login_page.go
│   │   │   ├── logout.go
│   │   │   ├── register.go
│   │   │   ├── register_test.go
│   │   │   ├── static.go
│   │   │   ├── static_test.go
│   │   │   └── support.go
│   │   ├── infrastructure/
│   │   │   └── postgresql/
│   │   │       ├── session_repository.go
│   │   │       └── session_repository_test.go
│   │   ├── middleware/
│   │   │   ├── middleware.go
│   │   │   └── middleware_test.go
│   │   └── ui/
│   │       ├── extra_render_test.go
│   │       ├── login.templ
│   │       ├── login_templ.go
│   │       └── login_templ_test.go
│   ├── editor/
│   │   ├── application/
│   │   ├── http/
│   │   │   ├── proxy.go
│   │   │   ├── proxy_test.go
│   │   │   ├── register.go
│   │   │   ├── view.go
│   │   │   └── view_test.go
│   │   └── ui/
│   │       ├── editor_view.templ
│   │       ├── editor_view_templ.go
│   │       ├── editor_view_templ_test.go
│   │       └── extra_render_test.go
│   ├── home/
│   │   ├── http/
│   │   │   ├── hello.go
│   │   │   ├── hello_test.go
│   │   │   └── register.go
│   │   └── ui/
│   │       ├── home.templ
│   │       ├── home_templ.go
│   │       └── home_templ_test.go
│   ├── quality/
│   │   ├── application/
│   │   │   └── test_report/
│   │   │       ├── runner.go
│   │   │       ├── runner_smoke_test.go
│   │   │       └── runner_test.go
│   │   ├── http/
│   │   │   ├── register.go
│   │   │   ├── register_test.go
│   │   │   ├── test_report_coverage.go
│   │   │   ├── test_report_coverage_test.go
│   │   │   ├── test_report_page.go
│   │   │   ├── test_report_page_test.go
│   │   │   ├── test_report_run.go
│   │   │   ├── test_report_run_test.go
│   │   │   ├── test_report_support.go
│   │   │   └── test_report_test.go
│   │   └── ui/
│   │       ├── extra_render_test.go
│   │       ├── fragments.templ
│   │       ├── fragments_templ.go
│   │       ├── page.templ
│   │       ├── page_templ.go
│   │       ├── page_templ_test.go
│   │       ├── render.go
│   │       ├── render_test.go
│   │       ├── state.go
│   │       └── state_test.go
│   ├── scheduler/
│   │   ├── application/
│   │   │   ├── http_client.go
│   │   │   ├── http_client_test.go
│   │   │   ├── job_config.go
│   │   │   ├── job_runner.go
│   │   │   └── job_runner_test.go
│   │   ├── http/
│   │   │   ├── jobs.go
│   │   │   ├── jobs_test.go
│   │   │   ├── register.go
│   │   │   ├── scheduler.go
│   │   │   └── scheduler_test.go
│   │   ├── infrastructure/
│   │   │   └── postgresql/
│   │   │       ├── job_repository.go
│   │   │       └── job_repository_test.go
│   │   └── ui/
│   │       ├── extra_render_test.go
│   │       ├── page.templ
│   │       ├── page_templ.go
│   │       └── page_templ_test.go
│   ├── shared/
│   │   ├── access.go
│   │   ├── access_test.go
│   │   ├── claims.go
│   │   ├── claims_test.go
│   │   ├── lifecycle.go
│   │   ├── lifecycle_test.go
│   │   ├── configuration/
│   │   │   ├── conf.go
│   │   │   ├── conf_test.go
│   │   │   ├── parse.go
│   │   │   ├── parse_extra_test.go
│   │   │   └── parse_test.go
│   │   ├── infrastructure/
│   │   │   ├── postgresql/
│   │   │   │   ├── connection.go
│   │   │   │   ├── connection_extra_test.go
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
│           ├── extra_render_test.go
│           ├── layout.templ
│           ├── layout_templ.go
│           ├── layout_templ_test.go
│           ├── layout_with_nav.templ
│           ├── layout_with_nav_templ.go
│           ├── render.go
│           ├── render_test.go
│           ├── sidenav.templ
│           ├── sidenav_templ.go
│           ├── types.go
│           ├── types_claims_test.go
│           └── types_test.go
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
