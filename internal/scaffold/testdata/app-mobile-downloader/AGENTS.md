# AGENTS

## Reglas del proyecto

### Frontend

Para cualquier tarea de frontend, páginas, layouts o componentes:

1. Ejecutar primero la skill `modern-web-guidance` antes de implementar HTML, CSS o JavaScript del cliente.
2. Usar la skill `daisyui` para cualquier generación de UI con HTML/JSX/Tailwind.
3. Usar `daisyui` como librería principal de componentes visuales. Preferir componentes y clases de daisyUI sobre markup Tailwind ad-hoc.
4. Si hay interacción server-driven, `hx-*`, swaps parciales, formularios dinámicos, SSE, WebSockets o integración con Go/templ, usar la skill `htmx`.
5. Cuando aplique `htmx`, preferir fragmentos HTML renderizados en servidor por sobre respuestas JSON y `fetch()` manual.
6. Antes de elegir un componente visual, revisar los componentes candidatos de daisyUI y seleccionar el más adecuado según la intención de la UI.
7. Mantener consistencia visual, accesibilidad y alta fidelidad en los componentes frontend.

### Convenciones de implementación

- Mantener nombres de archivos en inglés.
- Mantener contenido descriptivo y documentación en español, salvo que el contexto técnico requiera inglés.
- Preferir soluciones simples, mantenibles y alineadas a las skills del proyecto.

### Orden de decisión recomendado para frontend

1. `modern-web-guidance`
2. `daisyui`
3. `htmx` (solo cuando la interacción lo requiera)

---

## Estructura del proyecto

> **La estructura real y viva del proyecto está en `STRUCTURE.md`.**
> **No confiar en el diagrama de esta sección para rutas de archivos.**

### Arquitectura del proyecto

El proyecto sigue una **arquitectura modular con capas** organizada por **bounded contexts** (feature-based layered architecture), con elementos de Clean Architecture. No es arquitectura hexagonal pura.

Está organizado por **módulos de negocio** dentro de `internal/`. Cada módulo es una vertical autónoma con separación de capas internas:

- `application/` — Lógica de negocio, casos de uso e **interfaces** (ej. `Repository`).
  - No importa `http`, `infrastructure` ni `ui`.
  - Define contratos que la infraestructura implementa (Inversión de Dependencias).
- `http/` — Handlers HTTP, entrada del sistema.
  - Orquesta llamadas a `application/`.
  - Devuelve páginas completas o fragmentos HTMX según el request.
- `infrastructure/` — Adaptadores de salida (ej. PostgreSQL).
  - Implementa las interfaces definidas en `application/`.
- `ui/` — Plantillas go/templ (`*.templ` y `*_templ.go`).
  - Componentes visuales sin lógica de negocio.
- `middleware/` — Middleware específico del módulo (ej. JWT para auth).

Código compartido: `internal/shared/` (configuración, server, infraestructura común).

**Módulos actuales (bounded contexts):**
- `internal/auth/` — Autenticación OIDC, sesiones, JWT
- `internal/editor/` — Proxy al editor upstream
- `internal/home/` — Página de inicio
- `internal/quality/` — Reporte de tests y cobertura
- `internal/scheduler/` — Configuración de jobs (placeholder)

### Flujo de autenticación

1. **Middleware JWT** (`internal/auth/middleware/middleware.go`) se aplica a todo el servidor.
2. Rutas públicas: `/auth/login`, `/auth/callback`, `/auth/logout`, `/manifest.json`, `/favicon.ico`, `/icon.svg`, `/icon-180.png`.
3. Rutas de editor (`/editor/*`, `/assets/*`, `/api/*`, `/report/*`, `/scheduler/*`, `/editor-view`) requieren email en `allowedEditorEmails`.
4. Para el resto, el email debe estar en `allowedAppEmails`.
5. Modo dev: `AUTH_DISABLED=true` + header `X-Dev-Sub` salta la autenticación.

### Design system y themes

1. La fuente de verdad visual del proyecto es `DESIGN.md`.
2. Los temas viven en `design/<theme>/DESIGN.md`.
3. El formato base debe seguir Google `DESIGN.md` lo más fielmente posible.
4. Extensiones locales solo cuando sea necesario y bajo `x-pi`.
5. Los `DESIGN.md` se parsean y resuelven a CSS runtime; no hay build step de Tailwind para temas.
6. El CSS del tema activo se sirve desde `/design/theme/{id}`.
7. La selección del tema ocurre únicamente en `/design`.
8. El tema activo se resuelve por cookie `design-theme`.
9. Páginas públicas y autenticadas deben respetar el tema activo, incluyendo login y layouts compartidos.
10. Antes de hardcodear colores, tipografías, radios, spacing o sombras, revisar si ya existen tokens en el sistema.
11. Preferir variables y tokens del sistema (`ResolvedTheme`, CSS variables runtime, aliases DaisyUI) por sobre valores visuales ad-hoc.
12. No registrar en `AGENTS.md` cuál es el theme activo actual; eso es estado runtime, no regla de proyecto.

### Flujo de templates (go/templ)

1. **Layout base** (`internal/ui/layout/layout.templ`):
   - Carga HTMX v2, DaisyUI v5, Tailwind CSS v4 vía CDN.
   - Define `data-theme="light"`.
   - Usa `children...` para contenido.

2. **Layout con navegación** (`internal/ui/layout/layout_with_nav.templ`):
   - Usa drawer de DaisyUI con sidebar colapsable.
   - Incluye `@SideNav(nav)` para renderizar el menú lateral.
   - Se usa vía `layout.RenderPage(c, "Título", contenido)`.

3. **Sidenav** (`internal/ui/layout/sidenav.templ`):
   - Renderiza el menú lateral con ítems según permisos (`nav.IsEditor`).
   - Ítems: Inicio, Editor (Console), Quality, Scheduler, Cerrar sesión.

4. **Página completa** (ej. `internal/dev/ui/page.templ`):
   - El handler usa `layout.RenderPage(c, "Título", ui.Page(state))`.
   - El template `.templ` NO debe envolver con `@Layout(...)`; es solo contenido.
   - Botón con `templ.Attributes` inyectando `hx-post`, `hx-target`, `hx-swap`, `hx-indicator`.

5. **Fragmento parcial** (ej. `internal/dev/ui/fragments.templ`):
   - Componente `templ` sin layout, para usar en swaps HTMX (`hx-target`).
   - El handler devuelve solo el fragmento, no la página completa, cuando detecta `HX-Request`.

### Reglas para agregar nuevos templates

1. Crear archivo `.templ` dentro del módulo correspondiente (`internal/<modulo>/ui/`).
2. Para layouts compartidos: `internal/ui/layout/`.
3. Para páginas: definir componente `templ` sin layout; el handler aplica `layout.RenderPage(...)`.
4. Para fragmentos HTMX: componente `templ` sin layout.
5. Inyectar `hx-*` vía `templ.Attributes` para mantener componentes desacoplados.
6. Ejecutar `templ generate` antes de compilar.
7. Nunca editar manualmente los archivos `*_templ.go`.

### Reglas para agregar nuevos handlers

1. Crear archivo en el módulo correspondiente (`internal/<modulo>/http/`).
2. Registrar con `ioc.Register(nombreHandler)`.
3. Inyectar `*server.Server` como dependencia.
4. Usar `fuego.Get`, `fuego.Post`, `fuego.Handle` según método.
5. Si usa HTMX, verificar `HX-Request` y devolver fragmentos en lugar de páginas completas.
6. Si es ruta de editor, agregar prefijo a `isEditorPath` en `middleware.go` si es necesario.

### Reglas para tests

1. Tests en `*_test.go` junto al código que prueban.
2. Ejecutar con `go test ./...`.
3. El endpoint `/report/tests` ejecuta `go test -coverprofile=... ./...` y genera reporte HTML.
4. El reporte se almacena en `tmp/coverage/` y se sirve vía `/report/tests/coverage.html`.

### Flujo de trabajo recomendado

1. Antes de crear UI: `npx skills check` para verificar skills actualizadas.
2. Antes de implementar HTML: `npx -y modern-web-guidance@latest search "<tema>"`.
3. Antes de usar componentes: revisar docs en `.agents/skills/daisyui/components/`.
4. Después de editar `.templ`: `templ generate && go build ./...`.
5. Antes de commit: `go test ./...` y verificar que compila.
6. Después de cambios estructurales: ejecutar `scripts/generate-structure.sh` para regenerar `STRUCTURE.md`.

---

## Skills del proyecto

- **daisyui**: Componentes visuales (instalado en `.agents/skills/daisyui`).
- **htmx**: Interacción server-driven (instalado en `.agents/skills/htmx`).
- **modern-web-guidance**: Mejores prácticas web (instalado en `.agents/skills/modern-web-guidance`).

Para actualizar o instalar skills:
```bash
npm_config_cache="./.npm-cache" npx -y skills add <owner/repo> --skill <skill> -y
```

---

## Módulo agente (pkg/agent)

Desde 2026-07-02 el agente vive fuera de `internal/` en `pkg/agent/`. La razón
es que para un boilerplate conviene poder opt-out sin tocar el resto del
wiring. Detalle completo en `doc/agent-runtime.md`.

### Capas

```
pkg/agent/
├── application/         ← AgentService (interfaz pública) + Manager (impl)
├── http/                ← handlers /agent/* (consumen AgentService)
├── infrastructure/
│   ├── pirpc/           ← spawn de pi, sandbox CWD, prompt timeout
│   ├── disk/            ← session store persistente en AGENT_SESSION_DIR
│   └── memory/          ← session store en RAM (fallback)
└── ui/                  ← templates templ del chat
```

### Reglas

1. El host habla con el agente sólo vía `agentapp.AgentService`. Nada de
   tocar `*agentapp.Manager` desde fuera del paquete.
2. El sandbox CWD se resuelve solo en `pirpc.resolveCWD`. Si el caller
   pasa un CWD vacío o `.`, el runner lo redirige a `tmp/agent-work/<sessionID>/`.
3. El `.air.toml` debe seguir excluyendo `tmp/` para que las ediciones
   del agente dentro del sandbox no disparen rebuilds.
4. Para apagar el agente sin tocar el código: `AGENT_ENABLED=false`.
   El host omite los endpoints `/agent/*` y no levanta `pi`.

### Opt-out limpio

Si un proyecto derivado no quiere agente, basta con:

```sh
# al ejecutar el servidor
AGENT_ENABLED=false ./bin/server
```

Y si quieren sacar el wiring del PATH:

```sh
# eliminar el bloque "agent" en cmd/api/main.go:
# - los imports de pkg/agent
# - la llamada a registerAgent(s, hooks, newAgentDeps())
```

No hay dependencias del agente que el resto del código acuse de recibo.

---

## Topología de tres procesos (a partir del 2026-07-02)

El boilerplate corre tres binarios en paralelo en dev/prod. Cumplen
12-factor IX (disposability) en conjunto y desacoplan los restart
cycles:

```
   browser
      ↓
   BFF  (cmd/bff/, :8000)        proxy inverso 'tonto'. Hand-built,
                                 frozen, NO tocado por air.
      ↓                ↓
   /agent/*          /*
      ↓                ↓
   agent-worker      web-server
   :18080            :8001
   (cmd/agent-worker/)(cmd/api/)
   hot-reload vía    hot-reload OK
   air (cambia       (cambia mucho;
   poco)             el grueso de
                     la app vive acá)
```

### Authentication (Opción A — producción)

Cada servicio valida JWT contra el IdP (Casdoor) independientemente.
**NO** propagamos un internal token HMAC: el BFF deja pasar el
`Authorization` header tal cual al upstream, y tanto web-server como
agent-worker corren su propio `JWTMiddleware` con `JWKS_URL`,
`OIDC_ISSUER`, `JWT_AUDIENCE`. Cero secretos compartidos. Si el BFF
se cae, los upstreams siguen sirviendo con JWT válido.

Detalles y justificación en `doc/agent-runtime.md` §14.

### Reglas

1. El BFF solo rutea. No tiene lógica de negocio, no tiene handlers,
   no tiene estado. Si lo abrís y ves más de ~50 líneas de código,
   algo está mal.
2. El web-server (cmd/api) es el grueso del boilerplate. Editar acá
   dispara hot-reload por air.
3. El agent-worker (cmd/agent-worker) sostiene la runtime del agente.
   Cambios acá sólo matan al worker, no al web-server.
4. Editar cmd/bff NO requiere reinicio del web-server o del worker.
   Hay que recompilar `./bin/bff` a mano.

### Arranque para dev

```sh
# compila y arranca los tres procesos
scripts/run-all.sh start

# muestra pidfiles + reachability de las dos URLs principales
scripts/run-all.sh status

# limpieza
scripts/run-all.sh stop
```

### Producción

1. Construir cada binario en su propio target del Dockerfile / build
   pipeline.
2. Como tres servicios separados en compose / k8s.
3. El BFF es estable: deployar con replace strategy `Recreate` o
   `RollingUpdate` con zero-downtime no es estrictamente necesario
   porque el binario no cambia seguido.

Cuando el ciclo del agente migre a worker (step 2), el web-server ya
no registrará handlers de /agent/*. La documentación de los pasos
siguientes está en `doc/agent-runtime.md` §11+.

