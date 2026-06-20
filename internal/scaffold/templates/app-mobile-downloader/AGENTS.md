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
