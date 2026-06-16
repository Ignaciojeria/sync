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

### Arquitectura hexagonal

```
app-mobile-downloader/
├── cmd/api/main.go                    # Punto de entrada. Carga dependencias vía IoC.
├── internal/
│   ├── adapter/in/web/               # Entrada HTTP (handlers)
│   │   ├── hello.go                  # Handler raíz "/"
│   │   ├── auth_login.go             # GET /auth/login, /auth/login/google
│   │   ├── auth_callback.go          # GET /auth/callback, /auth/logout
│   │   ├── wede.go                   # Proxy /editor/* → wede upstream
│   │   └── test_report.go            # GET|POST /report/tests/* (tests + cobertura)
│   ├── shared/
│   │   ├── access/                   # Allowlists (editor/app emails)
│   │   ├── configuration/            # Parseo de env vars
│   │   ├── jwks/                     # Fetch de JWKS
│   │   ├── server/                   # Setup de Fuego + middleware
│   │   │   └── middleware/
│   │   │       └── middleware.go     # JWT, session cookie, auth paths
│   │   └── infrastructure/
│   │       └── postgresql/           # Conexión y migraciones
├── templates/                         # Plantillas go/templ
│   ├── layout.templ                   # Layout base (DaisyUI + HTMX)
│   ├── test_report.templ              # Página de reporte de tests
│   └── *_templ.go                     # Auto-generados por `templ generate`
└── .agents/skills/                    # Skills del proyecto (daisyui, htmx, modern-web-guidance)
```

### Flujo de autenticación

1. **Middleware JWT** (`middleware.go`) se aplica a todo el servidor.
2. Rutas públicas: `/auth/login`, `/auth/callback`, `/auth/logout`, `/manifest.json`, `/favicon.ico`, `/icon.svg`, `/icon-180.png`.
3. Rutas de editor (`/editor/*`, `/assets/*`, `/api/*`, `/report/*`) requieren email en `allowedEditorEmails`.
4. Para el resto, el email debe estar en `allowedAppEmails`.
5. Modo dev: `AUTH_DISABLED=true` + header `X-Dev-Sub` salta la autenticación.

### Flujo de templates (go/templ)

1. **Layout base** (`templates/layout.templ`):
   - Carga HTMX v2, DaisyUI v5, Tailwind CSS v4 vía CDN.
   - Define `data-theme="light"`.
   - Usa `children...` para contenido.

2. **Página completa** (`templates/test_report.templ`):
   - `@Layout("Título") { ... }` para render full page.
   - Botón con `templ.Attributes` inyectando `hx-post`, `hx-target`, `hx-swap`, `hx-indicator`.

3. **Fragmento parcial** (`templates/test_report.templ`):
   - `templ TestResult(success bool, output string, coverPath string)` se usa para swaps HTMX.
   - El handler `POST /report/tests/run` devuelve solo este fragmento, no la página completa.

### Reglas para agregar nuevos templates

1. Crear archivo `.templ` en `templates/`.
2. Para páginas: `@Layout("Título") { ... }`.
3. Para fragmentos HTMX: componente `templ` sin layout, para usar en `hx-target`.
4. Inyectar `hx-*` vía `templ.Attributes` para mantener componentes desacoplados.
5. Ejecutar `templ generate` antes de compilar.
6. Nunca editar manualmente los archivos `*_templ.go`.

### Reglas para agregar nuevos handlers

1. Crear archivo en `internal/adapter/in/web/`.
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

---

## Skills del proyecto

- **daisyui**: Componentes visuales (instalado en `.agents/skills/daisyui`).
- **htmx**: Interacción server-driven (instalado en `.agents/skills/htmx`).
- **modern-web-guidance**: Mejores prácticas web (instalado en `.agents/skills/modern-web-guidance`).

Para actualizar o instalar skills:
```bash
npm_config_cache="./.npm-cache" npx -y skills add <owner/repo> --skill <skill> -y
```
