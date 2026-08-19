# Estructura del Proyecto

> **Archivo generado automáticamente.** Ejecutar `scripts/generate-structure.sh` para regenerar.
> **No editar manualmente.**

```
.
├── .air.toml
├── AGENTS.md
├── STRUCTURE.md
├── go.mod
├── agents/
│   └── develop/
│       └── AGENTS.md
├── cmd/
│   └── api/
│       ├── main.go
│       └── main_test.go
├── design/
│   ├── _schema.md
│   ├── embed.go
│   ├── forest/
│   │   └── DESIGN.md
│   ├── ocean/
│   │   └── DESIGN.md
│   └── sunset/
│       └── DESIGN.md
├── internal/
│   ├── agent/
│   │   ├── application/
│   │   │   ├── assistant_text.go
│   │   │   ├── assistant_text_test.go
│   │   │   ├── event.go
│   │   │   ├── history.go
│   │   │   ├── history_100_test.go
│   │   │   ├── history_extra_test.go
│   │   │   ├── history_test.go
│   │   │   ├── manager.go
│   │   │   ├── manager_agentid_test.go
│   │   │   ├── manager_apply_test.go
│   │   │   ├── manager_merge_test.go
│   │   │   ├── manager_preview_test.go
│   │   │   ├── manager_sandbox_smoke_test.go
│   │   │   ├── manager_test.go
│   │   │   ├── memory.go
│   │   │   ├── memory_integration_test.go
│   │   │   ├── memory_test.go
│   │   │   ├── process_cleanup_linux.go
│   │   │   ├── process_cleanup_linux_test.go
│   │   │   ├── process_cleanup_other.go
│   │   │   ├── registry.go
│   │   │   ├── render.go
│   │   │   ├── render_registry_test.go
│   │   │   ├── render_streaming_test.go
│   │   │   ├── runner.go
│   │   │   ├── runtime_inventory.go
│   │   │   ├── runtime_inventory_test.go
│   │   │   └── session.go
│   │   ├── http/
│   │   │   ├── abort.go
│   │   │   ├── abort_test.go
│   │   │   ├── agent_e2e_isolated_test.go
│   │   │   ├── agent_e2e_test.go
│   │   │   ├── agent_test.go
│   │   │   ├── auth.go
│   │   │   ├── auth_extra_test.go
│   │   │   ├── auth_test.go
│   │   │   ├── email.go
│   │   │   ├── events.go
│   │   │   ├── events_sse_test.go
│   │   │   ├── merge.go
│   │   │   ├── merge_test.go
│   │   │   ├── page.go
│   │   │   ├── page_smoke_test.go
│   │   │   ├── page_test.go
│   │   │   ├── preview_context.go
│   │   │   ├── preview_context_test.go
│   │   │   ├── preview_context_ui.go
│   │   │   ├── preview_context_ui_test.go
│   │   │   ├── preview_proxy.go
│   │   │   ├── preview_proxy_test.go
│   │   │   ├── preview_register.go
│   │   │   ├── prompt.go
│   │   │   ├── register.go
│   │   │   ├── sessions.go
│   │   │   ├── sessions_backend_e2e_test.go
│   │   │   ├── support.go
│   │   │   ├── support_test.go
│   │   │   └── worktree_inspector.go
│   │   ├── infrastructure/
│   │   │   ├── disk/
│   │   │   │   └── session_store.go
│   │   │   ├── honcho/
│   │   │   │   ├── README.md
│   │   │   │   ├── adapter.go
│   │   │   │   ├── adapter_test.go
│   │   │   │   ├── client.go
│   │   │   │   ├── client_test.go
│   │   │   │   ├── keys.go
│   │   │   │   ├── keys_test.go
│   │   │   │   └── types.go
│   │   │   ├── memory/
│   │   │   │   └── session_store.go
│   │   │   ├── pirpc/
│   │   │   │   ├── process.go
│   │   │   │   ├── process_dedup_test.go
│   │   │   │   ├── process_test.go
│   │   │   │   ├── reader.go
│   │   │   │   ├── runner.go
│   │   │   │   ├── runner_test.go
│   │   │   │   ├── sandbox.go
│   │   │   │   └── sandbox_test.go
│   │   │   ├── preview/
│   │   │   │   └── launcher.go
│   │   │   └── worktree/
│   │   │       ├── inspector.go
│   │   │       ├── manager.go
│   │   │       └── manager_test.go
│   │   └── ui/
│   │       └── v2/
│   │           ├── embed.go
│   │           ├── fragments.templ
│   │           ├── fragments_templ.go
│   │           ├── htmlexport.go
│   │           ├── page.templ
│   │           ├── page_templ.go
│   │           ├── render_register.go
│   │           ├── standalone.templ
│   │           ├── standalone_templ.go
│   │           ├── state.go
│   │           ├── v2_test.go
│   │           └── static/
│   │               └── agent-chat/
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
│   ├── backlog/
│   │   ├── SPEC.md
│   │   ├── spec.go
│   │   ├── application/
│   │   │   ├── board.go
│   │   │   ├── board_test.go
│   │   │   ├── card.go
│   │   │   ├── dogfood_validate_test.go
│   │   │   ├── errors.go
│   │   │   ├── memstore_test.go
│   │   │   ├── parser.go
│   │   │   ├── parser_test.go
│   │   │   ├── service.go
│   │   │   ├── service_test.go
│   │   │   ├── writer.go
│   │   │   └── writer_test.go
│   │   ├── board/
│   │   │   ├── AGENTS.md
│   │   │   ├── index.md
│   │   │   ├── backlog/
│   │   │   │   ├── colapsar-tool-results-y-thinking-en-chat-v2.md
│   │   │   │   ├── dispatch-cards-a-otros-agentes-via-http.md
│   │   │   │   ├── ejemplo-ping-endpoint-de-healthcheck.md
│   │   │   │   ├── separar-raiz-del-agente-develop-a-agents-develop.md
│   │   │   │   └── supervisor-reactivo-opt-in-para-sesiones-del-agente.md
│   │   │   ├── done/
│   │   │   │   ├── deprecar-agent-v1-y-dejar-solo-v2.md
│   │   │   │   ├── enrutar-prompts-del-agente-por-honcho-para-reducir-consumo-d.md
│   │   │   │   ├── mejorar-el-detalle-del-backlog-agregando-seccion-de-plan.md
│   │   │   │   └── mover-los-md-del-backlog-de-tmp-a-internal-backlog.md
│   │   │   ├── in_progress/
│   │   │   └── todo/
│   │   ├── http/
│   │   │   ├── cards_delete.go
│   │   │   ├── cards_detail.go
│   │   │   ├── cards_move.go
│   │   │   ├── cards_priority.go
│   │   │   ├── cards_update.go
│   │   │   ├── page.go
│   │   │   ├── register.go
│   │   │   └── support.go
│   │   ├── infrastructure/
│   │   │   └── fs/
│   │   │       ├── lock.go
│   │   │       ├── store.go
│   │   │       └── store_test.go
│   │   └── ui/
│   │       ├── detail.templ
│   │       ├── detail_sections.go
│   │       ├── detail_sections.templ
│   │       ├── detail_sections_templ.go
│   │       ├── detail_sections_test.go
│   │       ├── detail_templ.go
│   │       ├── fragments.templ
│   │       ├── fragments_templ.go
│   │       ├── page.templ
│   │       ├── page_templ.go
│   │       ├── render.go
│   │       └── state.go
│   ├── design/
│   │   ├── application/
│   │   │   ├── catalog.go
│   │   │   ├── catalog_extra_test.go
│   │   │   ├── catalog_test.go
│   │   │   ├── document.go
│   │   │   ├── parser.go
│   │   │   ├── parser_test.go
│   │   │   ├── resolver.go
│   │   │   ├── resolver_test.go
│   │   │   ├── theme_css.go
│   │   │   └── theme_css_test.go
│   │   ├── http/
│   │   │   ├── page.go
│   │   │   ├── register.go
│   │   │   ├── select.go
│   │   │   └── theme_css.go
│   │   └── ui/
│   │       ├── page.templ
│   │       ├── page_templ.go
│   │       ├── page_templ_test.go
│   │       ├── panel.go
│   │       ├── panel.templ
│   │       ├── panel_templ.go
│   │       └── panel_templ_test.go
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
│   ├── gateway/
│   │   ├── application/
│   │   │   ├── balance.go
│   │   │   ├── balance_test.go
│   │   │   ├── session_cost.go
│   │   │   └── session_cost_test.go
│   │   ├── http/
│   │   │   └── balance.go
│   │   └── ui/
│   │       ├── balance.templ
│   │       └── balance_templ.go
│   ├── home/
│   │   ├── http/
│   │   │   ├── hello.go
│   │   │   ├── hello_test.go
│   │   │   └── register.go
│   │   └── ui/
│   │       ├── helpers.go
│   │       ├── helpers_test.go
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
│   │   │   │       ├── 000002_create_job_configs_and_logs.up.sql
│   │   │   │       ├── 000003_create_agent_runtime_events.down.sql
│   │   │   │       └── 000003_create_agent_runtime_events.up.sql
│   │   │   └── test/
│   │   │       ├── support.go
│   │   │       └── support_test.go
│   │   ├── jwks/
│   │   │   ├── new.go
│   │   │   └── new_test.go
│   │   ├── mounted/
│   │   │   ├── mounted.go
│   │   │   └── mounted_test.go
│   │   └── server/
│   │       ├── compat.go
│   │       ├── new.go
│   │       └── new_test.go
│   ├── topology/
│   │   ├── application/
│   │   │   ├── model.go
│   │   │   ├── service.go
│   │   │   ├── service_test.go
│   │   │   ├── sync_sessions.go
│   │   │   └── sync_sessions_test.go
│   │   ├── http/
│   │   │   ├── register.go
│   │   │   ├── sync_sessions.go
│   │   │   └── sync_sessions_test.go
│   │   └── infrastructure/
│   │       ├── memory/
│   │       │   ├── sync_sessions.go
│   │       │   └── sync_sessions_test.go
│   │       ├── merged/
│   │       │   ├── source.go
│   │       │   ├── source_overlay_test.go
│   │       │   └── source_test.go
│   │       ├── mutagen/
│   │       │   ├── source.go
│   │       │   └── source_test.go
│   │       ├── postgresql/
│   │       │   ├── source.go
│   │       │   └── source_test.go
│   │       └── workspacefiles/
│   │           ├── source.go
│   │           └── source_test.go
│   ├── ui/
│   │   └── layout/
│   │       ├── extra_render_test.go
│   │       ├── layout.templ
│   │       ├── layout_templ.go
│   │       ├── layout_templ_test.go
│   │       ├── layout_with_nav.templ
│   │       ├── layout_with_nav_templ.go
│   │       ├── preview_merge_bar.templ
│   │       ├── preview_merge_bar_helpers_test.go
│   │       ├── preview_merge_bar_templ.go
│   │       ├── preview_merge_bar_test.go
│   │       ├── render.go
│   │       ├── render_test.go
│   │       ├── sidenav.templ
│   │       ├── sidenav_templ.go
│   │       ├── types.go
│   │       ├── types_claims_test.go
│   │       └── types_test.go
│   └── versions/
│       ├── application/
│       │   ├── git_history.go
│       │   ├── git_history_test.go
│       │   └── types.go
│       ├── http/
│       │   ├── handler.go
│       │   ├── handler_test.go
│       │   └── register.go
│       └── ui/
│           ├── page.templ
│           ├── page_templ.go
│           └── state.go
└── scripts/
    ├── _tree_generator.py
    ├── check-mounted-paths.sh
    ├── generate-structure.sh
    ├── reset-agent-runtime.sh
    ├── run-api.sh
    ├── structure.config.toml
    ├── test-honcho-conversation.sh
    └── test-honcho-recall.sh
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
